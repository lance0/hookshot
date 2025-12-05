package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/lance0/hookshot/internal/protocol"
)

// Config holds server configuration
type Config struct {
	Port           int
	Host           string
	PublicURL      string
	MaxRequests    int
	Token          string   // Optional: require this token for auth
	TLSCert        string   // Optional: path to TLS certificate
	TLSKey         string   // Optional: path to TLS key
	MaxBodySize    int64    // Max webhook body size in bytes (default 10MB)
	MaxMessageSize int64    // Max WebSocket message size in bytes (default 10MB)
	AllowedOrigins []string // Optional: allowed WebSocket origins (empty = allow all for CLI clients)
}

const (
	defaultMaxBodySize    = 10 * 1024 * 1024 // 10MB
	defaultMaxMessageSize = 10 * 1024 * 1024 // 10MB
)

// Server is the hookshot relay server
type Server struct {
	config   Config
	registry *TunnelRegistry
	store    *RequestStore
	upgrader websocket.Upgrader
}

// New creates a new server
func New(cfg Config) *Server {
	// Apply defaults
	if cfg.MaxBodySize == 0 {
		cfg.MaxBodySize = defaultMaxBodySize
	}
	if cfg.MaxMessageSize == 0 {
		cfg.MaxMessageSize = defaultMaxMessageSize
	}

	store := NewRequestStore(cfg.MaxRequests)
	s := &Server{
		config:   cfg,
		registry: NewTunnelRegistry(store),
		store:    store,
	}

	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     s.checkOrigin,
	}

	return s
}

// checkOrigin validates WebSocket connection origins
func (s *Server) checkOrigin(r *http.Request) bool {
	// If no origins configured, allow all (needed for CLI clients with no Origin header)
	if len(s.config.AllowedOrigins) == 0 {
		return true
	}

	origin := r.Header.Get("Origin")
	// CLI clients typically don't send Origin header
	if origin == "" {
		return true
	}

	// Check against allowed origins
	for _, allowed := range s.config.AllowedOrigins {
		if origin == allowed {
			return true
		}
	}

	log.Printf("rejected WebSocket connection from origin: %s", origin)
	return false
}

// Run starts the server with graceful shutdown support
func (s *Server) Run(ctx context.Context) error {
	r := mux.NewRouter()

	// WebSocket endpoint for clients
	r.HandleFunc("/ws", s.handleWebSocket)

	// API endpoints (protected by auth if token is set)
	api := r.PathPrefix("/api").Subrouter()
	if s.config.Token != "" {
		api.Use(s.authMiddleware)
	}
	api.HandleFunc("/tunnels/{tunnel_id}/requests", s.handleListRequests).Methods("GET")
	api.HandleFunc("/tunnels/{tunnel_id}/requests/{request_id}/replay", s.handleReplay).Methods("POST")

	// Webhook endpoints - catch all methods and paths under /t/{tunnel_id}
	// Note: webhooks are NOT auth-protected (external services need to reach them)
	r.PathPrefix("/t/{tunnel_id}").HandlerFunc(s.handleWebhook)

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	if s.config.PublicURL != "" {
		log.Printf("public URL: %s", s.config.PublicURL)
	}
	if s.config.Token != "" {
		log.Printf("auth token required for connections")
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if s.config.TLSCert != "" && s.config.TLSKey != "" {
			log.Printf("hookshot server listening on %s (TLS)", addr)
			errCh <- srv.ListenAndServeTLS(s.config.TLSCert, s.config.TLSKey)
		} else {
			log.Printf("hookshot server listening on %s", addr)
			errCh <- srv.ListenAndServe()
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		log.Printf("shutting down server...")
		// Give 10 seconds to drain connections
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Close all tunnels gracefully
		s.registry.CloseAll()

		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// authMiddleware checks for valid auth token
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.checkAuth(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// checkAuth validates the auth token from Authorization header only
func (s *Server) checkAuth(r *http.Request) bool {
	if s.config.Token == "" {
		return true
	}

	// Check Authorization header (Bearer token)
	auth := r.Header.Get("Authorization")
	if auth != "" {
		if len(auth) > 7 && auth[:7] == "Bearer " {
			if auth[7:] == s.config.Token {
				return true
			}
		}
	}

	// Query param tokens removed for security (leak risk in logs/proxies)
	return false
}

// handleWebSocket handles client WebSocket connections
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	// Set message size limit
	conn.SetReadLimit(s.config.MaxMessageSize)

	// Wait for register message
	_, message, err := conn.ReadMessage()
	if err != nil {
		log.Printf("failed to read register message: %v", err)
		conn.Close()
		return
	}

	var msg protocol.Message
	if err := json.Unmarshal(message, &msg); err != nil || msg.Type != protocol.TypeRegister {
		log.Printf("expected register message, got: %s", msg.Type)
		conn.Close()
		return
	}

	var regPayload protocol.RegisterPayload
	if err := msg.ParsePayload(&regPayload); err != nil {
		log.Printf("failed to parse register payload: %v", err)
		conn.Close()
		return
	}

	// Check auth token if required
	if s.config.Token != "" && regPayload.Token != s.config.Token {
		log.Printf("unauthorized connection attempt")
		errMsg, _ := protocol.NewMessage(protocol.TypeError, protocol.ErrorPayload{
			Code:    "unauthorized",
			Message: "invalid or missing auth token",
		})
		data, _ := json.Marshal(errMsg)
		conn.WriteMessage(websocket.TextMessage, data)
		conn.Close()
		return
	}

	tunnel, err := s.registry.Register(conn, regPayload.TunnelID)
	if err != nil {
		log.Printf("failed to register tunnel: %v", err)
		conn.Close()
		return
	}

	// Send registered confirmation
	publicURL := s.config.PublicURL
	if publicURL == "" {
		publicURL = fmt.Sprintf("http://%s:%d", s.config.Host, s.config.Port)
	}

	registeredMsg, _ := protocol.NewMessage(protocol.TypeRegistered, protocol.RegisteredPayload{
		TunnelID:  tunnel.ID,
		PublicURL: fmt.Sprintf("%s/t/%s", publicURL, tunnel.ID),
	})
	data, _ := json.Marshal(registeredMsg)
	conn.WriteMessage(websocket.TextMessage, data)

	log.Printf("tunnel registered: %s", tunnel.ShortID())

	// Start read/write pumps
	go tunnel.WritePump()
	tunnel.ReadPump(s.registry)

	log.Printf("tunnel disconnected: %s", tunnel.ShortID())
}

// handleWebhook handles incoming webhook requests
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tunnelID := vars["tunnel_id"]

	tunnel, ok := s.registry.Get(tunnelID)
	if !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}

	// Read the request body with size limit
	r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if err.Error() == "http: request body too large" {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Build the path (everything after /t/{tunnel_id})
	path := r.URL.Path[len("/t/"+tunnelID):]
	if path == "" {
		path = "/"
	}
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}

	// Create the request
	req := &protocol.HTTPRequest{
		ID:        uuid.New().String()[:8],
		TunnelID:  tunnelID,
		Method:    r.Method,
		Path:      path,
		Headers:   protocol.HeadersFromHTTP(r.Header),
		Body:      body,
		Timestamp: time.Now(),
	}

	// Store the request
	s.store.Store(tunnelID, req)

	// Forward to client
	ctx, cancel := context.WithTimeout(r.Context(), responseWait)
	defer cancel()

	resp, err := tunnel.ForwardRequest(ctx, req)
	if err != nil {
		log.Printf("[%s] forward error (tunnel=%s, method=%s, path=%s): %v",
			req.ID, tunnel.ShortID(), req.Method, req.Path, err)
		http.Error(w, fmt.Sprintf("failed to forward request (id=%s)", req.ID), http.StatusBadGateway)
		return
	}

	// Write response back
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(resp.Body)
}

// handleListRequests lists recent requests for a tunnel
func (s *Server) handleListRequests(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tunnelID := vars["tunnel_id"]

	requests := s.store.List(tunnelID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(requests)
}

// handleReplay replays a request
func (s *Server) handleReplay(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tunnelID := vars["tunnel_id"]
	requestID := vars["request_id"]

	tunnel, ok := s.registry.Get(tunnelID)
	if !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}

	req, ok := s.store.Get(requestID)
	if !ok {
		http.Error(w, "request not found", http.StatusNotFound)
		return
	}

	// Verify the request belongs to this tunnel
	if req.TunnelID != tunnelID {
		http.Error(w, "request not found", http.StatusNotFound)
		return
	}

	// Create a new request with a new ID for replay
	replayReq := &protocol.HTTPRequest{
		ID:        uuid.New().String()[:8],
		TunnelID:  tunnelID,
		Method:    req.Method,
		Path:      req.Path,
		Headers:   req.Headers,
		Body:      req.Body,
		Timestamp: time.Now(),
	}

	// Store the replay request
	s.store.Store(tunnelID, replayReq)

	// Forward to client
	ctx, cancel := context.WithTimeout(r.Context(), responseWait)
	defer cancel()

	resp, err := tunnel.ForwardRequest(ctx, replayReq)
	if err != nil {
		log.Printf("[%s] replay error (tunnel=%s, original=%s): %v",
			replayReq.ID, tunnel.ShortID(), requestID, err)
		http.Error(w, fmt.Sprintf("failed to replay request (id=%s)", replayReq.ID), http.StatusBadGateway)
		return
	}

	// Return the response as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"request_id":  replayReq.ID,
		"status_code": resp.StatusCode,
		"headers":     resp.Headers,
		"body_length": len(resp.Body),
	})
}
