package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lance0/hookshot/internal/protocol"
)

// TargetResolver resolves the target URL for a given path
type TargetResolver func(path string) string

// Forwarder forwards requests to a local target
type Forwarder struct {
	defaultTarget  string
	targetResolver TargetResolver
	httpClient     *http.Client
}

// NewForwarder creates a new forwarder with a single default target
func NewForwarder(target string) *Forwarder {
	return &Forwarder{
		defaultTarget:  target,
		targetResolver: nil,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			// Don't follow redirects automatically
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// NewForwarderWithRoutes creates a forwarder with route-based target resolution
func NewForwarderWithRoutes(defaultTarget string, resolver TargetResolver) *Forwarder {
	return &Forwarder{
		defaultTarget:  defaultTarget,
		targetResolver: resolver,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// resolveTarget gets the target for a path
func (f *Forwarder) resolveTarget(path string) string {
	if f.targetResolver != nil {
		return f.targetResolver(path)
	}
	return f.defaultTarget
}

// Forward forwards a request to the local target and returns the response
func (f *Forwarder) Forward(ctx context.Context, req *protocol.HTTPRequest) (*protocol.HTTPResponse, error) {
	// Resolve target based on path
	target := f.resolveTarget(req.Path)

	// Build the full URL
	url := target + req.Path

	// Create the HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, bytes.NewReader(req.Body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Copy headers
	for k, v := range req.Headers {
		// Skip hop-by-hop headers
		if isHopByHop(k) {
			continue
		}
		httpReq.Header.Set(k, v)
	}

	// Make the request
	resp, err := f.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to forward request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Build response headers (skip hop-by-hop)
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if isHopByHop(k) {
			continue
		}
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	return &protocol.HTTPResponse{
		RequestID:  req.ID,
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
	}, nil
}

// isHopByHop returns true if the header is a hop-by-hop header
func isHopByHop(header string) bool {
	switch header {
	case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"Te", "Trailers", "Transfer-Encoding", "Upgrade":
		return true
	}
	return false
}
