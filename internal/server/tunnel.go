package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/lance0/hookshot/internal/protocol"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	responseWait   = 30 * time.Second
)

// Tunnel represents a connected client tunnel
type Tunnel struct {
	ID        string
	conn      *websocket.Conn
	send      chan []byte
	pending   map[string]chan *protocol.HTTPResponse // requestID -> response channel
	pendingMu sync.Mutex
	done      chan struct{}
}

// TunnelRegistry manages active tunnels
type TunnelRegistry struct {
	mu      sync.RWMutex
	tunnels map[string]*Tunnel
	store   *RequestStore
}

// NewTunnelRegistry creates a new tunnel registry
func NewTunnelRegistry(store *RequestStore) *TunnelRegistry {
	return &TunnelRegistry{
		tunnels: make(map[string]*Tunnel),
		store:   store,
	}
}

// Register registers a new tunnel with optional requested ID
func (r *TunnelRegistry) Register(conn *websocket.Conn, requestedID string) (*Tunnel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tunnelID := requestedID
	if tunnelID == "" {
		tunnelID = uuid.New().String()[:8]
	} else if _, exists := r.tunnels[tunnelID]; exists {
		// Requested ID is taken, generate a new one
		tunnelID = uuid.New().String()[:8]
	}

	tunnel := &Tunnel{
		ID:      tunnelID,
		conn:    conn,
		send:    make(chan []byte, 256),
		pending: make(map[string]chan *protocol.HTTPResponse),
		done:    make(chan struct{}),
	}
	r.tunnels[tunnelID] = tunnel
	return tunnel, nil
}

// Unregister removes a tunnel from the registry
func (r *TunnelRegistry) Unregister(tunnelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if tunnel, ok := r.tunnels[tunnelID]; ok {
		close(tunnel.done)
		close(tunnel.send)
		delete(r.tunnels, tunnelID)
	}
}

// Get retrieves a tunnel by ID
func (r *TunnelRegistry) Get(tunnelID string) (*Tunnel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tunnels[tunnelID]
	return t, ok
}

// ForwardRequest sends a request through the tunnel and waits for response
func (t *Tunnel) ForwardRequest(ctx context.Context, req *protocol.HTTPRequest) (*protocol.HTTPResponse, error) {
	respChan := make(chan *protocol.HTTPResponse, 1)

	t.pendingMu.Lock()
	t.pending[req.ID] = respChan
	t.pendingMu.Unlock()

	defer func() {
		t.pendingMu.Lock()
		delete(t.pending, req.ID)
		t.pendingMu.Unlock()
	}()

	msg, err := protocol.NewMessage(protocol.TypeRequest, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create message: %w", err)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	select {
	case t.send <- data:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.done:
		return nil, fmt.Errorf("tunnel closed")
	}

	select {
	case resp := <-respChan:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.done:
		return nil, fmt.Errorf("tunnel closed")
	}
}

// HandleResponse processes an incoming response from the client
func (t *Tunnel) HandleResponse(resp *protocol.HTTPResponse) {
	t.pendingMu.Lock()
	ch, ok := t.pending[resp.RequestID]
	t.pendingMu.Unlock()

	if ok {
		select {
		case ch <- resp:
		default:
		}
	}
}

// WritePump pumps messages from the send channel to the WebSocket connection
func (t *Tunnel) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		t.conn.Close()
	}()

	for {
		select {
		case message, ok := <-t.send:
			t.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				t.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := t.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			t.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := t.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-t.done:
			return
		}
	}
}

// ReadPump pumps messages from the WebSocket connection
func (t *Tunnel) ReadPump(registry *TunnelRegistry) {
	defer func() {
		registry.Unregister(t.ID)
		t.conn.Close()
	}()

	t.conn.SetReadDeadline(time.Now().Add(pongWait))
	t.conn.SetPongHandler(func(string) error {
		t.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := t.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("tunnel %s read error: %v", t.ID, err)
			}
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("tunnel %s: failed to parse message: %v", t.ID, err)
			continue
		}

		switch msg.Type {
		case protocol.TypeResponse:
			var resp protocol.HTTPResponse
			if err := msg.ParsePayload(&resp); err != nil {
				log.Printf("tunnel %s: failed to parse response: %v", t.ID, err)
				continue
			}
			t.HandleResponse(&resp)
			registry.store.StoreResponse(&resp)
		case protocol.TypePong:
			// Client responded to ping, connection is alive
		default:
			log.Printf("tunnel %s: unknown message type: %s", t.ID, msg.Type)
		}
	}
}
