package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	ServerURL string
	Token     string
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
	statusMsg     string
	statusTime    time.Time

	// Filter mode
	filterMode  bool
	filterInput string

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
type replayResultMsg struct {
	success   bool
	requestID string
	message   string
}

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

func (m Model) replayRequest(requestID string) tea.Cmd {
	return func() tea.Msg {
		if m.connection.ServerURL == "" || m.connection.TunnelID == "" {
			return replayResultMsg{success: false, requestID: requestID, message: "Not connected"}
		}

		url := fmt.Sprintf("%s/api/tunnels/%s/requests/%s/replay",
			m.connection.ServerURL, m.connection.TunnelID, requestID)

		req, err := http.NewRequest("POST", url, nil)
		if err != nil {
			return replayResultMsg{success: false, requestID: requestID, message: err.Error()}
		}
		req.Header.Set("Content-Type", "application/json")
		if m.connection.Token != "" {
			req.Header.Set("Authorization", "Bearer "+m.connection.Token)
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return replayResultMsg{success: false, requestID: requestID, message: err.Error()}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return replayResultMsg{success: false, requestID: requestID, message: fmt.Sprintf("Server returned %d", resp.StatusCode)}
		}

		var result struct {
			RequestID string `json:"request_id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return replayResultMsg{success: true, requestID: requestID, message: "Replayed"}
		}

		return replayResultMsg{success: true, requestID: requestID, message: fmt.Sprintf("Replayed → %s", result.RequestID)}
	}
}

// filteredRequests returns requests matching the current filter
func (m Model) filteredRequests() []RequestItem {
	if m.filterInput == "" {
		return m.requests
	}
	filter := strings.ToLower(m.filterInput)
	var filtered []RequestItem
	for _, req := range m.requests {
		if strings.Contains(strings.ToLower(req.Path), filter) ||
			strings.Contains(strings.ToLower(req.Method), filter) ||
			strings.Contains(req.ID, filter) {
			filtered = append(filtered, req)
		}
	}
	return filtered
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle filter mode input
		if m.filterMode {
			switch msg.Type {
			case tea.KeyEsc:
				m.filterMode = false
				m.filterInput = ""
				m.selected = 0
			case tea.KeyEnter:
				m.filterMode = false
			case tea.KeyBackspace:
				if len(m.filterInput) > 0 {
					m.filterInput = m.filterInput[:len(m.filterInput)-1]
					m.selected = 0
				}
			default:
				if msg.Type == tea.KeyRunes {
					m.filterInput += string(msg.Runes)
					m.selected = 0
				}
			}
			return m, tea.Batch(cmds...)
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, m.keys.Up):
			if m.selected > 0 {
				m.selected--
			}

		case key.Matches(msg, m.keys.Down):
			filtered := m.filteredRequests()
			if m.selected < len(filtered)-1 {
				m.selected++
			}

		case key.Matches(msg, m.keys.Filter):
			m.filterMode = true

		case key.Matches(msg, m.keys.Clear):
			m.filterInput = ""
			m.selected = 0

		case key.Matches(msg, m.keys.Replay):
			filtered := m.filteredRequests()
			if len(filtered) > 0 && m.selected < len(filtered) {
				req := filtered[m.selected]
				m.statusMsg = fmt.Sprintf("Replaying %s...", req.ID)
				m.statusTime = time.Now()
				cmds = append(cmds, m.replayRequest(req.ID))
			}
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
		// Clear status message after 3 seconds
		if m.statusMsg != "" && time.Since(m.statusTime) > 3*time.Second {
			m.statusMsg = ""
		}

	case replayResultMsg:
		if msg.success {
			m.statusMsg = SuccessStyle.Render("✓ ") + msg.message
		} else {
			m.statusMsg = ErrorStyle.Render("✗ ") + msg.message
		}
		m.statusTime = time.Now()
	}

	// Update viewport content
	filtered := m.filteredRequests()
	if len(filtered) > 0 && m.selected < len(filtered) {
		m.viewport.SetContent(m.renderDetail(filtered[m.selected]))
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

	// Show filter or replay hint
	var rightSide string
	if m.filterMode {
		rightSide = DimStyle.Render("filter: ") + lipgloss.NewStyle().Foreground(Sky).Render(m.filterInput) + lipgloss.NewStyle().Foreground(Sky).Blink(true).Render("▎")
	} else if m.filterInput != "" {
		rightSide = DimStyle.Render("filter: ") + lipgloss.NewStyle().Foreground(Sky).Render(m.filterInput) + "  " + DimStyle.Render("[esc]clear")
	} else {
		rightSide = DimStyle.Render("[r]eplay [/]filter")
	}
	headerLine := header + strings.Repeat(" ", max(0, m.width-lipgloss.Width(header)-lipgloss.Width(rightSide)-6)) + rightSide

	var rows []string
	rows = append(rows, headerLine)
	rows = append(rows, DimStyle.Render(strings.Repeat("─", m.width-6)))

	filtered := m.filteredRequests()
	if len(m.requests) == 0 {
		rows = append(rows, DimStyle.Render("  Waiting for requests..."))
	} else if len(filtered) == 0 {
		rows = append(rows, DimStyle.Render("  No matching requests"))
	} else {
		// Show up to 8 requests
		maxRows := min(8, len(filtered))
		for i := 0; i < maxRows; i++ {
			rows = append(rows, m.renderRequestRow(i, filtered[i]))
		}
		if len(filtered) > maxRows {
			rows = append(rows, DimStyle.Render(fmt.Sprintf("  ... and %d more", len(filtered)-maxRows)))
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

	filtered := m.filteredRequests()
	var content string
	if len(filtered) > 0 && m.selected < len(filtered) {
		content = headerLine + "\n" + DimStyle.Render(strings.Repeat("─", m.width-6)) + "\n" + m.viewport.View()
	} else {
		content = headerLine + "\n" + DimStyle.Render(strings.Repeat("─", m.width-6)) + "\n" + DimStyle.Render("  Select a request to view details")
	}

	return DetailBoxStyle.Width(m.width - 2).Render(content)
}

func (m Model) renderHelp() string {
	if m.statusMsg != "" {
		return "  " + m.statusMsg
	}
	if m.filterMode {
		return "  " + DimStyle.Render("Type to filter • Enter to confirm • Esc to cancel")
	}
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
