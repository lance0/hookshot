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
	Port        int
	Host        string
	PublicURL   string
	MaxRequests int
}

// Server is the hookshot relay server
type Server struct {
	config   Config
	registry *TunnelRegistry
	store    *RequestStore
	upgrader websocket.Upgrader
}

// New creates a new server
func New(cfg Config) *Server {
	store := NewRequestStore(cfg.MaxRequests)
	return &Server{
		config:   cfg,
		registry: NewTunnelRegistry(store),
		store:    store,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}
}

// Run starts the server
func (s *Server) Run() error {
	r := mux.NewRouter()

	// WebSocket endpoint for clients
	r.HandleFunc("/ws", s.handleWebSocket)

	// API endpoints
	r.HandleFunc("/api/tunnels/{tunnel_id}/requests", s.handleListRequests).Methods("GET")
	r.HandleFunc("/api/tunnels/{tunnel_id}/requests/{request_id}/replay", s.handleReplay).Methods("POST")

	// Webhook endpoints - catch all methods and paths under /t/{tunnel_id}
	r.PathPrefix("/t/{tunnel_id}").HandlerFunc(s.handleWebhook)

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	log.Printf("hookshot server listening on %s", addr)
	if s.config.PublicURL != "" {
		log.Printf("public URL: %s", s.config.PublicURL)
	}
	return http.ListenAndServe(addr, r)
}

// handleWebSocket handles client WebSocket connections
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

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

	log.Printf("tunnel registered: %s", tunnel.ID)

	// Start read/write pumps
	go tunnel.WritePump()
	tunnel.ReadPump(s.registry)

	log.Printf("tunnel disconnected: %s", tunnel.ID)
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

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
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
		log.Printf("forward error for %s: %v", req.ID, err)
		http.Error(w, "failed to forward request", http.StatusBadGateway)
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

	// Create a new request with a new ID for replay
	replayReq := &protocol.HTTPRequest{
		ID:        uuid.New().String()[:8],
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
		log.Printf("replay error for %s: %v", replayReq.ID, err)
		http.Error(w, "failed to replay request", http.StatusBadGateway)
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
