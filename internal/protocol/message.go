package protocol

import (
	"encoding/json"
	"net/http"
	"time"
)

// Message types for WebSocket communication
const (
	TypeRegister   = "register"
	TypeRegistered = "registered"
	TypeRequest    = "request"
	TypeResponse   = "response"
	TypePing       = "ping"
	TypePong       = "pong"
	TypeError      = "error"
)

// Message is the envelope for all WebSocket messages
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// RegisterPayload is sent by client to register a tunnel
type RegisterPayload struct {
	TunnelID string `json:"tunnel_id,omitempty"` // Optional: client-requested ID
}

// RegisteredPayload is sent by server to confirm registration
type RegisteredPayload struct {
	TunnelID  string `json:"tunnel_id"`
	PublicURL string `json:"public_url"`
}

// HTTPRequest represents an incoming webhook request to be forwarded
type HTTPRequest struct {
	ID        string            `json:"id"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Headers   map[string]string `json:"headers"`
	Body      []byte            `json:"body"`
	Timestamp time.Time         `json:"timestamp"`
}

// HTTPResponse represents the response from the local server
type HTTPResponse struct {
	RequestID  string            `json:"request_id"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
}

// ErrorPayload represents an error message
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewMessage creates a new message with the given type and payload
func NewMessage(msgType string, payload interface{}) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Message{
		Type:    msgType,
		Payload: data,
	}, nil
}

// ParsePayload parses the message payload into the given type
func (m *Message) ParsePayload(v interface{}) error {
	return json.Unmarshal(m.Payload, v)
}

// HeadersFromHTTP converts http.Header to a simple map
func HeadersFromHTTP(h http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

// HeadersToHTTP converts a simple map back to http.Header
func HeadersToHTTP(h map[string]string) http.Header {
	result := make(http.Header)
	for k, v := range h {
		result.Set(k, v)
	}
	return result
}
