package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RequestItem represents a webhook request/response pair
type RequestItem struct {
	ID         string
	Method     string
	Path       string
	StatusCode int
	Duration   time.Duration
	Timestamp  time.Time
	ReqHeaders map[string]string
	ReqBody    []byte
	ResHeaders map[string]string
	ResBody    []byte
	Error      string
}

// ConnectionInfo holds tunnel connection details
type ConnectionInfo struct {
	TunnelID  string
	PublicURL string
	Target    string
	Connected bool
}

// Model is the main TUI model
type Model struct {
	requests      []RequestItem
	selected      int
	keys          KeyMap
	width         int
	height        int
	viewport      viewport.Model
	viewportReady bool
	connection    ConnectionInfo
	ready         bool
	quitting      bool

	// Channels for communication
	requestCh chan RequestItem
	connCh    chan ConnectionInfo
}

// NewModel creates a new TUI model
func NewModel() Model {
	return Model{
		requests:  make([]RequestItem, 0),
		selected:  0,
		keys:      DefaultKeyMap,
		requestCh: make(chan RequestItem, 100),
		connCh:    make(chan ConnectionInfo, 1),
	}
}

// RequestChannel returns the channel for sending requests to the TUI
func (m *Model) RequestChannel() chan<- RequestItem {
	return m.requestCh
}

// ConnectionChannel returns the channel for sending connection info
func (m *Model) ConnectionChannel() chan<- ConnectionInfo {
	return m.connCh
}

// Messages
type requestMsg RequestItem
type connectionMsg ConnectionInfo
type tickMsg time.Time

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.waitForRequest(),
		m.waitForConnection(),
		m.tick(),
	)
}

func (m Model) waitForRequest() tea.Cmd {
	return func() tea.Msg {
		return requestMsg(<-m.requestCh)
	}
}

func (m Model) waitForConnection() tea.Cmd {
	return func() tea.Msg {
		return connectionMsg(<-m.connCh)
	}
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, m.keys.Up):
			if m.selected > 0 {
				m.selected--
			}

		case key.Matches(msg, m.keys.Down):
			if m.selected < len(m.requests)-1 {
				m.selected++
			}

		case key.Matches(msg, m.keys.Replay):
			// TODO: Implement replay
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Update viewport size
		headerHeight := 6
		listHeight := min(10, m.height/3)
		detailHeight := m.height - headerHeight - listHeight - 4

		if !m.viewportReady {
			m.viewport = viewport.New(m.width-4, detailHeight)
			m.viewport.YPosition = 0
			m.viewportReady = true
		} else {
			m.viewport.Width = m.width - 4
			m.viewport.Height = detailHeight
		}

	case requestMsg:
		// Prepend new request (newest first)
		m.requests = append([]RequestItem{RequestItem(msg)}, m.requests...)
		// Keep max 100 requests
		if len(m.requests) > 100 {
			m.requests = m.requests[:100]
		}
		cmds = append(cmds, m.waitForRequest())

	case connectionMsg:
		m.connection = ConnectionInfo(msg)
		cmds = append(cmds, m.waitForConnection())

	case tickMsg:
		// Refresh for relative timestamps
		cmds = append(cmds, m.tick())
	}

	// Update viewport content
	if len(m.requests) > 0 && m.selected < len(m.requests) {
		m.viewport.SetContent(m.renderDetail(m.requests[m.selected]))
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if !m.ready {
		return "\n  Initializing..."
	}

	var b strings.Builder

	// Header
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	// Request list
	b.WriteString(m.renderList())
	b.WriteString("\n")

	// Detail view
	b.WriteString(m.renderDetailBox())
	b.WriteString("\n")

	// Help
	b.WriteString(m.renderHelp())

	return b.String()
}

func (m Model) renderHeader() string {
	title := IconStyle.Render("🎯") + " " + TitleStyle.Render("hookshot")

	var status string
	if m.connection.Connected {
		status = SuccessStyle.Render("●") + " " + DimStyle.Render("connected")
	} else {
		status = ErrorStyle.Render("●") + " " + DimStyle.Render("disconnected")
	}

	tunnelInfo := ""
	if m.connection.TunnelID != "" {
		tunnelInfo = DimStyle.Render("tunnel: ") + lipgloss.NewStyle().Foreground(Lavender).Render(m.connection.TunnelID)
	}

	// First line: title and tunnel
	titleLine := lipgloss.JoinHorizontal(
		lipgloss.Left,
		title,
		strings.Repeat(" ", max(0, m.width-lipgloss.Width(title)-lipgloss.Width(tunnelInfo)-lipgloss.Width(status)-8)),
		tunnelInfo,
		"  ",
		status,
	)

	// Connection info
	urlLine := ""
	if m.connection.PublicURL != "" {
		urlLine = DimStyle.Render("  Public URL: ") + URLStyle.Render(m.connection.PublicURL)
	}

	targetLine := ""
	if m.connection.Target != "" {
		targetLine = DimStyle.Render("  Forwarding: ") + lipgloss.NewStyle().Foreground(Green).Render(m.connection.Target)
	}

	content := titleLine
	if urlLine != "" {
		content += "\n" + urlLine
	}
	if targetLine != "" {
		content += "\n" + targetLine
	}

	return HeaderBoxStyle.Width(m.width - 2).Render(content)
}

func (m Model) renderList() string {
	header := SectionStyle.Render("REQUESTS")
	replayHint := DimStyle.Render("[r]eplay")
	headerLine := header + strings.Repeat(" ", max(0, m.width-lipgloss.Width(header)-lipgloss.Width(replayHint)-6)) + replayHint

	var rows []string
	rows = append(rows, headerLine)
	rows = append(rows, DimStyle.Render(strings.Repeat("─", m.width-6)))

	if len(m.requests) == 0 {
		rows = append(rows, DimStyle.Render("  Waiting for requests..."))
	} else {
		// Show up to 8 requests
		maxRows := min(8, len(m.requests))
		for i := 0; i < maxRows; i++ {
			rows = append(rows, m.renderRequestRow(i, m.requests[i]))
		}
		if len(m.requests) > maxRows {
			rows = append(rows, DimStyle.Render(fmt.Sprintf("  ... and %d more", len(m.requests)-maxRows)))
		}
	}

	content := strings.Join(rows, "\n")
	return ListBoxStyle.Width(m.width - 2).Render(content)
}

func (m Model) renderRequestRow(index int, req RequestItem) string {
	// Selection indicator
	indicator := "  "
	if index == m.selected {
		indicator = IconStyle.Render("▸ ")
	}

	// Method
	method := MethodStyle(req.Method).Width(7).Render(req.Method)

	// Path (truncate if needed)
	maxPathLen := m.width - 50
	path := req.Path
	if len(path) > maxPathLen {
		path = path[:maxPathLen-3] + "..."
	}

	// Status
	var status string
	if req.StatusCode > 0 {
		status = StatusStyle(req.StatusCode).Width(4).Render(fmt.Sprintf("%d", req.StatusCode))
	} else if req.Error != "" {
		status = ErrorStyle.Width(4).Render("ERR")
	} else {
		status = DimStyle.Width(4).Render("...")
	}

	// Duration
	duration := DimStyle.Width(6).Render(formatDuration(req.Duration))

	// Relative time
	relTime := DimStyle.Width(10).Render(relativeTime(req.Timestamp))

	// ID
	id := DimStyle.Render(req.ID)

	row := fmt.Sprintf("%s%s %s %s %s %s %s",
		indicator, method, path,
		strings.Repeat(" ", max(0, maxPathLen-len(req.Path))),
		status, duration, relTime+" "+id)

	if index == m.selected {
		return SelectedStyle.Width(m.width - 6).Render(row)
	}
	return row
}

func (m Model) renderDetail(req RequestItem) string {
	var b strings.Builder

	// Request line
	b.WriteString(MethodStyle(req.Method).Render(req.Method))
	b.WriteString(" ")
	b.WriteString(lipgloss.NewStyle().Foreground(Text).Render(req.Path))
	b.WriteString("\n")

	// Request headers
	if len(req.ReqHeaders) > 0 {
		b.WriteString(DimStyle.Render(strings.Repeat("─", 40)))
		b.WriteString("\n")
		for k, v := range req.ReqHeaders {
			if k == "Content-Type" || k == "User-Agent" || k == "X-Request-Id" {
				b.WriteString(DimStyle.Render(k+": "))
				b.WriteString(lipgloss.NewStyle().Foreground(Subtext0).Render(v))
				b.WriteString("\n")
			}
		}
	}

	// Request body
	if len(req.ReqBody) > 0 {
		b.WriteString(DimStyle.Render(strings.Repeat("─", 40)))
		b.WriteString("\n")
		body := truncateBody(req.ReqBody, 500)
		b.WriteString(lipgloss.NewStyle().Foreground(Text).Render(body))
		b.WriteString("\n")
	}

	// Response
	b.WriteString(DimStyle.Render(strings.Repeat("─", 40)))
	b.WriteString("\n")

	if req.Error != "" {
		b.WriteString(ErrorStyle.Render("Error: " + req.Error))
	} else if req.StatusCode > 0 {
		b.WriteString(DimStyle.Render("Response: "))
		b.WriteString(StatusStyle(req.StatusCode).Render(fmt.Sprintf("%d", req.StatusCode)))
		b.WriteString(DimStyle.Render(fmt.Sprintf(" (%s)", formatDuration(req.Duration))))
		b.WriteString("\n")

		if len(req.ResBody) > 0 {
			body := truncateBody(req.ResBody, 500)
			b.WriteString(lipgloss.NewStyle().Foreground(Subtext0).Render(body))
		}
	} else {
		b.WriteString(DimStyle.Render("Pending..."))
	}

	return b.String()
}

func (m Model) renderDetailBox() string {
	header := SectionStyle.Render("REQUEST DETAIL")
	headerLine := header

	var content string
	if len(m.requests) > 0 && m.selected < len(m.requests) {
		content = headerLine + "\n" + DimStyle.Render(strings.Repeat("─", m.width-6)) + "\n" + m.viewport.View()
	} else {
		content = headerLine + "\n" + DimStyle.Render(strings.Repeat("─", m.width-6)) + "\n" + DimStyle.Render("  Select a request to view details")
	}

	return DetailBoxStyle.Width(m.width - 2).Render(content)
}

func (m Model) renderHelp() string {
	help := "  " + DimStyle.Render("↑↓ navigate  r replay  / filter  q quit")
	return help
}

// Helper functions

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(d.Hours()))
}

func truncateBody(body []byte, maxLen int) string {
	s := string(body)
	// Replace newlines for compact display
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
