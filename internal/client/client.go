package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lance0/hookshot/internal/protocol"
	"github.com/lance0/hookshot/internal/tui"
)

const (
	reconnectDelay    = 2 * time.Second
	maxReconnectDelay = 30 * time.Second
	pongWait          = 60 * time.Second
)

// Route maps a path prefix to a target
type Route struct {
	Path   string
	Target string
}

// Config holds client configuration
type Config struct {
	ServerURL string
	Target    string  // Default target
	Routes    []Route // Optional: route by path
	TunnelID  string  // Optional: requested tunnel ID
	Token     string  // Optional: auth token
	Verbose   bool    // Show request/response bodies
	TUIMode   bool    // Enable TUI mode
}

// Client is the hookshot tunnel client
type Client struct {
	config    Config
	forwarder *Forwarder
	display   *Display
	conn      *websocket.Conn
	tunnelID  string
	publicURL string

	// TUI mode channels
	tuiRequestCh chan<- tui.RequestItem
	tuiConnCh    chan<- tui.ConnectionInfo
}

// New creates a new client
func New(cfg Config) *Client {
	var forwarder *Forwarder

	if len(cfg.Routes) > 0 {
		// Create forwarder with route-based resolution
		forwarder = NewForwarderWithRoutes(cfg.Target, func(path string) string {
			return matchRoute(cfg.Routes, cfg.Target, path)
		})
	} else {
		forwarder = NewForwarder(cfg.Target)
	}

	return &Client{
		config:    cfg,
		forwarder: forwarder,
		display:   NewDisplay(cfg.Target, cfg.Verbose),
	}
}

// matchRoute finds the best matching route for a path
func matchRoute(routes []Route, defaultTarget, path string) string {
	var bestMatch Route
	bestLen := -1

	for _, route := range routes {
		if strings.HasPrefix(path, route.Path) && len(route.Path) > bestLen {
			bestMatch = route
			bestLen = len(route.Path)
		}
	}

	if bestLen >= 0 {
		return bestMatch.Target
	}
	return defaultTarget
}

// Run connects to the server and starts forwarding requests
func (c *Client) Run(ctx context.Context) error {
	attempt := 0
	delay := reconnectDelay

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := c.connect(ctx)
		if err != nil {
			c.display.LogDisconnected(err)

			attempt++
			c.display.LogReconnecting(attempt)

			select {
			case <-time.After(delay):
				delay = min(delay*2, maxReconnectDelay)
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}

		// Reset reconnect state on successful connection
		attempt = 0
		delay = reconnectDelay

		err = c.runLoop(ctx)
		if err != nil {
			c.display.LogDisconnected(err)

			if ctx.Err() != nil {
				return ctx.Err()
			}

			// Reconnect
			c.display.LogReconnecting(1)
			time.Sleep(reconnectDelay)
		}
	}
}

// connect establishes a WebSocket connection to the server
func (c *Client) connect(ctx context.Context) error {
	// Parse the server URL and convert to WebSocket
	serverURL := c.config.ServerURL
	u, err := url.Parse(serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	// Add /ws path
	u.Path = "/ws"

	// Convert http(s) to ws(s) if needed
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}

	// Connect
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	c.conn = conn

	// Send register message
	regPayload := protocol.RegisterPayload{
		TunnelID: c.config.TunnelID,
		Token:    c.config.Token,
	}
	msg, _ := protocol.NewMessage(protocol.TypeRegister, regPayload)
	data, _ := json.Marshal(msg)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		conn.Close()
		return fmt.Errorf("failed to send register: %w", err)
	}

	// Wait for registered response
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to read register response: %w", err)
	}
	conn.SetReadDeadline(time.Time{})

	var respMsg protocol.Message
	if err := json.Unmarshal(message, &respMsg); err != nil {
		conn.Close()
		return fmt.Errorf("invalid register response: %w", err)
	}

	if respMsg.Type == protocol.TypeError {
		var errPayload protocol.ErrorPayload
		respMsg.ParsePayload(&errPayload)
		conn.Close()
		return fmt.Errorf("server error: %s", errPayload.Message)
	}

	if respMsg.Type != protocol.TypeRegistered {
		conn.Close()
		return fmt.Errorf("unexpected response type: %s", respMsg.Type)
	}

	var registered protocol.RegisteredPayload
	if err := respMsg.ParsePayload(&registered); err != nil {
		conn.Close()
		return fmt.Errorf("invalid registered payload: %w", err)
	}

	c.tunnelID = registered.TunnelID
	c.publicURL = registered.PublicURL
	c.display.LogConnected(c.tunnelID, c.publicURL)

	// Send connection info to TUI if enabled
	if c.tuiConnCh != nil {
		c.tuiConnCh <- tui.ConnectionInfo{
			TunnelID:  c.tunnelID,
			PublicURL: c.publicURL,
			Target:    c.config.Target,
			ServerURL: c.config.ServerURL,
			Token:     c.config.Token,
			Connected: true,
		}
	}

	return nil
}

// runLoop handles incoming messages
func (c *Client) runLoop(ctx context.Context) error {
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			c.conn.Close()
			return ctx.Err()
		default:
		}

		_, message, err := c.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}

		var msg protocol.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case protocol.TypeRequest:
			var req protocol.HTTPRequest
			if err := msg.ParsePayload(&req); err != nil {
				continue
			}
			go c.handleRequest(ctx, &req)

		case protocol.TypePing:
			// Respond with pong
			pongMsg, _ := protocol.NewMessage(protocol.TypePong, nil)
			data, _ := json.Marshal(pongMsg)
			c.conn.WriteMessage(websocket.TextMessage, data)
		}
	}
}

// handleRequest forwards a request to the local target
func (c *Client) handleRequest(ctx context.Context, req *protocol.HTTPRequest) {
	c.display.LogRequest(req)

	start := time.Now()

	// Forward the request
	resp, err := c.forwarder.Forward(ctx, req)
	duration := time.Since(start)

	var errMsg string
	if err != nil {
		c.display.LogError(req, err)
		errMsg = err.Error()
		// Send error response
		resp = &protocol.HTTPResponse{
			RequestID:  req.ID,
			StatusCode: 502,
			Headers:    map[string]string{"Content-Type": "text/plain"},
			Body:       []byte(fmt.Sprintf("Failed to forward: %v", err)),
		}
	} else {
		c.display.LogResponse(req, resp, duration)
	}

	// Send to TUI if enabled
	if c.tuiRequestCh != nil {
		tuiReq := tui.RequestItem{
			ID:         req.ID,
			Method:     req.Method,
			Path:       req.Path,
			StatusCode: resp.StatusCode,
			Duration:   duration,
			Timestamp:  time.Now(),
			ReqHeaders: req.Headers,
			ReqBody:    req.Body,
			ResHeaders: resp.Headers,
			ResBody:    resp.Body,
			Error:      errMsg,
		}
		select {
		case c.tuiRequestCh <- tuiReq:
		default:
			// Don't block if channel is full
		}
	}

	// Send response back
	msg, _ := protocol.NewMessage(protocol.TypeResponse, resp)
	data, _ := json.Marshal(msg)
	c.conn.WriteMessage(websocket.TextMessage, data)
}

// GetTunnelID returns the current tunnel ID
func (c *Client) GetTunnelID() string {
	return c.tunnelID
}

// GetPublicURL returns the public URL
func (c *Client) GetPublicURL() string {
	return c.publicURL
}

// SetTUIChannels sets channels for TUI communication
func (c *Client) SetTUIChannels(reqCh chan<- tui.RequestItem, connCh chan<- tui.ConnectionInfo) {
	c.tuiRequestCh = reqCh
	c.tuiConnCh = connCh
}

// GetTarget returns the target URL
func (c *Client) GetTarget() string {
	return c.config.Target
}
