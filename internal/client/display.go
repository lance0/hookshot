package client

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/lance0/hookshot/internal/protocol"
)

var (
	methodColors = map[string]*color.Color{
		"GET":     color.New(color.FgGreen),
		"POST":    color.New(color.FgYellow),
		"PUT":     color.New(color.FgBlue),
		"DELETE":  color.New(color.FgRed),
		"PATCH":   color.New(color.FgMagenta),
		"OPTIONS": color.New(color.FgCyan),
		"HEAD":    color.New(color.FgWhite),
	}
	defaultMethodColor = color.New(color.FgWhite)

	statusColors = map[int]*color.Color{
		2: color.New(color.FgGreen),  // 2xx
		3: color.New(color.FgCyan),   // 3xx
		4: color.New(color.FgYellow), // 4xx
		5: color.New(color.FgRed),    // 5xx
	}
	defaultStatusColor = color.New(color.FgWhite)

	dimColor    = color.New(color.Faint)
	arrowColor  = color.New(color.FgCyan)
	idColor     = color.New(color.FgHiBlack)
)

// Display handles request/response logging
type Display struct {
	target string
}

// NewDisplay creates a new display
func NewDisplay(target string) *Display {
	return &Display{target: target}
}

// LogRequest logs an incoming request
func (d *Display) LogRequest(req *protocol.HTTPRequest) {
	timestamp := time.Now().Format("15:04:05")

	methodColor := methodColors[req.Method]
	if methodColor == nil {
		methodColor = defaultMethodColor
	}

	// Format: [15:04:05] → POST /webhooks/stripe (abc123)
	fmt.Printf("%s %s %s %s %s\n",
		dimColor.Sprintf("[%s]", timestamp),
		arrowColor.Sprint("→"),
		methodColor.Sprintf("%-7s", req.Method),
		req.Path,
		idColor.Sprintf("(%s)", req.ID),
	)
}

// LogResponse logs a response
func (d *Display) LogResponse(req *protocol.HTTPRequest, resp *protocol.HTTPResponse, duration time.Duration) {
	timestamp := time.Now().Format("15:04:05")

	statusColor := statusColors[resp.StatusCode/100]
	if statusColor == nil {
		statusColor = defaultStatusColor
	}

	// Format: [15:04:05] ← 200 OK (15ms)
	fmt.Printf("%s %s %s %s\n",
		dimColor.Sprintf("[%s]", timestamp),
		arrowColor.Sprint("←"),
		statusColor.Sprintf("%d", resp.StatusCode),
		dimColor.Sprintf("(%s)", formatDuration(duration)),
	)
}

// LogError logs an error
func (d *Display) LogError(req *protocol.HTTPRequest, err error) {
	timestamp := time.Now().Format("15:04:05")

	fmt.Printf("%s %s %s\n",
		dimColor.Sprintf("[%s]", timestamp),
		color.RedString("✗"),
		color.RedString("error: %v", err),
	)
}

// LogConnected logs successful connection
func (d *Display) LogConnected(tunnelID, publicURL string) {
	fmt.Println()
	color.Green("✓ Connected!")
	fmt.Println()
	fmt.Printf("  Tunnel ID:  %s\n", color.CyanString(tunnelID))
	fmt.Printf("  Public URL: %s\n", color.CyanString(publicURL))
	fmt.Printf("  Forwarding: %s\n", color.CyanString(d.target))
	fmt.Println()
	fmt.Println(dimColor.Sprint("  Waiting for requests..."))
	fmt.Println(strings.Repeat("─", 50))
}

// LogDisconnected logs disconnection
func (d *Display) LogDisconnected(err error) {
	if err != nil {
		color.Yellow("\n⚠ Disconnected: %v", err)
	} else {
		color.Yellow("\n⚠ Disconnected")
	}
}

// LogReconnecting logs reconnection attempt
func (d *Display) LogReconnecting(attempt int) {
	color.Yellow("↻ Reconnecting (attempt %d)...", attempt)
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
