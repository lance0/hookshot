package server

import (
	"sync"

	"github.com/lance0/hookshot/internal/protocol"
)

const defaultMaxRequests = 100

// RequestStore stores request history for replay functionality
type RequestStore struct {
	mu          sync.RWMutex
	requests    map[string]*protocol.HTTPRequest    // requestID -> request
	byTunnel    map[string][]string                 // tunnelID -> []requestID (ordered)
	responses   map[string]*protocol.HTTPResponse   // requestID -> response
	maxRequests int
}

// NewRequestStore creates a new request store
func NewRequestStore(maxRequests int) *RequestStore {
	if maxRequests <= 0 {
		maxRequests = defaultMaxRequests
	}
	return &RequestStore{
		requests:    make(map[string]*protocol.HTTPRequest),
		byTunnel:    make(map[string][]string),
		responses:   make(map[string]*protocol.HTTPResponse),
		maxRequests: maxRequests,
	}
}

// Store stores a request for a tunnel
func (s *RequestStore) Store(tunnelID string, req *protocol.HTTPRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests[req.ID] = req
	s.byTunnel[tunnelID] = append(s.byTunnel[tunnelID], req.ID)

	// Evict old requests if over limit
	if len(s.byTunnel[tunnelID]) > s.maxRequests {
		oldID := s.byTunnel[tunnelID][0]
		s.byTunnel[tunnelID] = s.byTunnel[tunnelID][1:]
		delete(s.requests, oldID)
		delete(s.responses, oldID)
	}
}

// StoreResponse stores the response for a request
func (s *RequestStore) StoreResponse(resp *protocol.HTTPResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses[resp.RequestID] = resp
}

// Get retrieves a request by ID
func (s *RequestStore) Get(requestID string) (*protocol.HTTPRequest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	req, ok := s.requests[requestID]
	return req, ok
}

// GetResponse retrieves a response by request ID
func (s *RequestStore) GetResponse(requestID string) (*protocol.HTTPResponse, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	resp, ok := s.responses[requestID]
	return resp, ok
}

// RequestSummary is a brief summary of a request for listing
type RequestSummary struct {
	ID         string `json:"id"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Timestamp  string `json:"timestamp"`
	StatusCode int    `json:"status_code,omitempty"`
}

// List returns summaries of requests for a tunnel (newest first)
func (s *RequestStore) List(tunnelID string) []RequestSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.byTunnel[tunnelID]
	result := make([]RequestSummary, 0, len(ids))

	// Return in reverse order (newest first)
	for i := len(ids) - 1; i >= 0; i-- {
		req := s.requests[ids[i]]
		if req == nil {
			continue
		}
		summary := RequestSummary{
			ID:        req.ID,
			Method:    req.Method,
			Path:      req.Path,
			Timestamp: req.Timestamp.Format("2006-01-02T15:04:05Z"),
		}
		if resp, ok := s.responses[req.ID]; ok {
			summary.StatusCode = resp.StatusCode
		}
		result = append(result, summary)
	}
	return result
}

// Clear removes all requests for a tunnel
func (s *RequestStore) Clear(tunnelID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range s.byTunnel[tunnelID] {
		delete(s.requests, id)
		delete(s.responses, id)
	}
	delete(s.byTunnel, tunnelID)
}
