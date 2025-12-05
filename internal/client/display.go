package client

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fatih/color"
	"github.com/lance0/hookshot/internal/protocol"
)

const (
	maxBodyDisplay = 500 // Max chars to display for body
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
	bodyColor   = color.New(color.FgHiBlack)
)

// Display handles request/response logging
type Display struct {
	target  string
	verbose bool
}

// NewDisplay creates a new display
func NewDisplay(target string, verbose bool) *Display {
	return &Display{target: target, verbose: verbose}
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

	// Show body in verbose mode
	if d.verbose && len(req.Body) > 0 {
		d.logBody("   req", req.Body)
	}
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

	// Show body in verbose mode
	if d.verbose && len(resp.Body) > 0 {
		d.logBody("   res", resp.Body)
	}
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

// logBody logs a truncated body with prefix
func (d *Display) logBody(prefix string, body []byte) {
	// Only display if it looks like text
	if !isTextBody(body) {
		fmt.Printf("%s %s\n", bodyColor.Sprint(prefix), dimColor.Sprintf("[binary %d bytes]", len(body)))
		return
	}

	s := string(body)
	// Clean up for display (single line, truncate)
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\t", " ")

	truncated := false
	if len(s) > maxBodyDisplay {
		s = s[:maxBodyDisplay]
		truncated = true
	}

	if truncated {
		fmt.Printf("%s %s%s\n", bodyColor.Sprint(prefix), bodyColor.Sprint(s), dimColor.Sprint("..."))
	} else {
		fmt.Printf("%s %s\n", bodyColor.Sprint(prefix), bodyColor.Sprint(s))
	}
}

// isTextBody checks if body appears to be text content
func isTextBody(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	// Check if it's valid UTF-8 and doesn't contain too many control chars
	if !utf8.Valid(body) {
		return false
	}
	// Sample first 512 bytes
	sample := body
	if len(sample) > 512 {
		sample = sample[:512]
	}
	controlChars := 0
	for _, b := range sample {
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			controlChars++
		}
	}
	// If more than 10% control chars, consider it binary
	return float64(controlChars)/float64(len(sample)) < 0.1
}
