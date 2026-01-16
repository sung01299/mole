package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mole-cli/mole/internal/ngrok"
	"github.com/mole-cli/mole/internal/tui/messages"
	"github.com/mole-cli/mole/internal/util"
)

// FocusState represents which panel is currently focused
type FocusState int

const (
	FocusList FocusState = iota
	FocusDetail
	FocusSidebar
)

// Polling intervals
const (
	ActivePollingInterval = 300 * time.Millisecond
	IdlePollingInterval   = 2 * time.Second
)

// App is the main Bubbletea model
type App struct {
	// Window dimensions
	width  int
	height int

	// Focus state
	focus FocusState

	// Data
	tunnels   []ngrok.Tunnel
	requests  []ngrok.Request
	selected  int
	lastError error

	// Components
	viewport viewport.Model
	spinner  spinner.Model
	keys     KeyMap

	// API client
	client *ngrok.Client

	// State
	loading     bool
	windowFocus bool
	ready       bool
}

// NewApp creates a new App instance
func NewApp(client *ngrok.Client) *App {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = SpinnerStyle

	return &App{
		client:      client,
		keys:        DefaultKeyMap(),
		spinner:     s,
		loading:     true,
		windowFocus: true,
		focus:       FocusList,
	}
}

// Init implements tea.Model
func (a *App) Init() tea.Cmd {
	return tea.Batch(
		a.spinner.Tick,
		a.fetchTunnels(),
		a.fetchRequests(),
		tickCmd(ActivePollingInterval),
	)
}

// Update implements tea.Model
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		cmd := a.handleKeyPress(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.updateViewportSize()
		a.ready = true

	case messages.TickMsg:
		interval := ActivePollingInterval
		if !a.windowFocus {
			interval = IdlePollingInterval
		}
		cmds = append(cmds, a.fetchRequests(), tickCmd(interval))

	case messages.TunnelsMsg:
		a.loading = false
		if msg.Err != nil {
			a.lastError = msg.Err
		} else {
			a.tunnels = msg.Tunnels
			a.lastError = nil
		}

	case messages.RequestsMsg:
		a.loading = false
		if msg.Err != nil {
			a.lastError = msg.Err
		} else {
			// Preserve selection if possible
			oldLen := len(a.requests)
			a.requests = msg.Requests
			if a.selected >= len(a.requests) {
				a.selected = max(0, len(a.requests)-1)
			}
			// Update detail view if we have new data
			if len(a.requests) != oldLen && a.focus == FocusDetail {
				a.updateDetailContent()
			}
			a.lastError = nil
		}

	case messages.ReplayMsg:
		if msg.Err != nil {
			a.lastError = msg.Err
		} else {
			// Refresh requests after replay
			cmds = append(cmds, a.fetchRequests())
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update viewport if in detail mode
	if a.focus == FocusDetail {
		var cmd tea.Cmd
		a.viewport, cmd = a.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}

// handleKeyPress processes key events
func (a *App) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, a.keys.Quit):
		return tea.Quit

	case key.Matches(msg, a.keys.Down):
		if a.focus == FocusList && len(a.requests) > 0 {
			a.selected = min(a.selected+1, len(a.requests)-1)
		}

	case key.Matches(msg, a.keys.Up):
		if a.focus == FocusList && len(a.requests) > 0 {
			a.selected = max(a.selected-1, 0)
		}

	case key.Matches(msg, a.keys.Top):
		if a.focus == FocusList {
			a.selected = 0
		}

	case key.Matches(msg, a.keys.Bottom):
		if a.focus == FocusList && len(a.requests) > 0 {
			a.selected = len(a.requests) - 1
		}

	case key.Matches(msg, a.keys.Enter):
		if a.focus == FocusList && len(a.requests) > 0 {
			a.focus = FocusDetail
			a.updateDetailContent()
		}

	case key.Matches(msg, a.keys.Escape):
		if a.focus == FocusDetail {
			a.focus = FocusList
		}

	case key.Matches(msg, a.keys.Replay):
		if len(a.requests) > 0 && a.selected < len(a.requests) {
			return a.replayRequest(a.requests[a.selected].ID)
		}
	}

	return nil
}

// View implements tea.Model
func (a *App) View() string {
	if !a.ready {
		return fmt.Sprintf("\n  %s Loading...", a.spinner.View())
	}

	// Build layout
	header := a.renderHeader()
	content := a.renderContent()
	footer := a.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

// renderHeader renders the top header with tunnel info
func (a *App) renderHeader() string {
	var tunnelInfo string
	if len(a.tunnels) > 0 {
		t := a.tunnels[0]
		tunnelInfo = fmt.Sprintf(" %s → %s ",
			TunnelURLStyle.Render(t.PublicURL),
			TunnelLocalStyle.Render(t.Config.Addr),
		)
	} else if a.lastError != nil {
		tunnelInfo = ErrorStyle.Render(" ⚠ ngrok not running ")
	} else {
		tunnelInfo = " No active tunnels "
	}

	title := HeaderStyle.Render(" 🕳 MOLE ")
	info := lipgloss.NewStyle().
		Background(lipgloss.Color("#1F2937")).
		Padding(0, 1).
		Render(tunnelInfo)

	headerContent := lipgloss.JoinHorizontal(lipgloss.Center, title, info)

	return lipgloss.NewStyle().
		Width(a.width).
		Render(headerContent)
}

// renderContent renders the main content area
func (a *App) renderContent() string {
	contentHeight := a.height - 4 // header + footer

	if a.focus == FocusDetail {
		return a.renderDetailFullscreen(contentHeight)
	}

	// Responsive layout
	if a.width >= 120 {
		return a.renderSideBySide(contentHeight)
	}
	return a.renderStacked(contentHeight)
}

// renderSideBySide renders list and detail side by side
func (a *App) renderSideBySide(height int) string {
	listWidth := a.width / 2
	detailWidth := a.width - listWidth

	list := a.renderRequestList(listWidth-2, height-2)
	detail := a.renderDetailPanel(detailWidth-2, height-2)

	listBox := ActiveBorderStyle.Width(listWidth - 2).Height(height - 2).Render(list)
	detailBox := BorderStyle.Width(detailWidth - 2).Height(height - 2).Render(detail)

	return lipgloss.JoinHorizontal(lipgloss.Top, listBox, detailBox)
}

// renderStacked renders list above detail
func (a *App) renderStacked(height int) string {
	listHeight := height * 60 / 100
	detailHeight := height - listHeight

	list := a.renderRequestList(a.width-4, listHeight-2)
	detail := a.renderDetailPanel(a.width-4, detailHeight-2)

	listBox := ActiveBorderStyle.Width(a.width - 4).Height(listHeight - 2).Render(list)
	detailBox := BorderStyle.Width(a.width - 4).Height(detailHeight - 2).Render(detail)

	return lipgloss.JoinVertical(lipgloss.Left, listBox, detailBox)
}

// renderRequestList renders the list of requests
func (a *App) renderRequestList(width, height int) string {
	if len(a.requests) == 0 {
		msg := "Waiting for requests..."
		if a.loading {
			msg = a.spinner.View() + " " + msg
		}
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, msg)
	}

	var lines []string
	title := ListTitleStyle.Render("Requests")
	lines = append(lines, title)

	visibleLines := height - 2

	// Calculate scroll offset
	startIdx := 0
	if a.selected >= visibleLines {
		startIdx = a.selected - visibleLines + 1
	}

	endIdx := min(startIdx+visibleLines, len(a.requests))

	for i := startIdx; i < endIdx; i++ {
		req := a.requests[i]
		line := a.renderRequestLine(req, width-2, i == a.selected)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// renderRequestLine renders a single request line
func (a *App) renderRequestLine(req ngrok.Request, width int, selected bool) string {
	method := MethodStyle.
		Foreground(MethodColor(req.Request.Method)).
		Render(fmt.Sprintf("%-6s", req.Request.Method))

	status := StatusStyle.
		Foreground(StatusCodeColor(req.StatusCode())).
		Render(fmt.Sprintf("%d", req.StatusCode()))

	duration := DurationStyle.Render(fmt.Sprintf("%6.0fms", req.DurationMs()))

	// Calculate remaining width for path
	fixedWidth := 6 + 1 + 3 + 1 + 8 // method + space + status + space + duration
	pathWidth := width - fixedWidth - 4

	path := util.TruncateString(req.Request.URI, pathWidth)
	pathStyled := PathStyle.Render(path)

	line := fmt.Sprintf("%s %s %s %s", method, status, pathStyled, duration)

	if selected {
		return SelectedItemStyle.Width(width).Render(line)
	}
	return NormalItemStyle.Width(width).Render(line)
}

// renderDetailPanel renders the detail panel (side panel mode)
func (a *App) renderDetailPanel(width, height int) string {
	if len(a.requests) == 0 || a.selected >= len(a.requests) {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
			"Select a request to view details")
	}

	req := a.requests[a.selected]
	return a.renderRequestDetail(req, width, height, false)
}

// renderDetailFullscreen renders the detail panel in fullscreen mode
func (a *App) renderDetailFullscreen(height int) string {
	if len(a.requests) == 0 || a.selected >= len(a.requests) {
		return ""
	}

	content := ActiveBorderStyle.
		Width(a.width - 4).
		Height(height - 2).
		Render(a.viewport.View())

	return content
}

// renderRequestDetail renders request details
func (a *App) renderRequestDetail(req ngrok.Request, width, height int, full bool) string {
	var sb strings.Builder

	// Title
	title := DetailTitleStyle.Render(fmt.Sprintf("%s %s", req.Request.Method, req.Request.URI))
	sb.WriteString(title + "\n\n")

	// Status and timing
	status := StatusStyle.
		Foreground(StatusCodeColor(req.StatusCode())).
		Render(req.ResponseStatus)
	sb.WriteString(fmt.Sprintf("%s %s  %s %.2fms\n\n",
		DetailLabelStyle.Render("Status:"), status,
		DetailLabelStyle.Render("Duration:"), req.DurationMs()))

	// Request headers
	sb.WriteString(DetailLabelStyle.Render("Request Headers:\n"))
	for key, values := range req.Request.Headers {
		for _, v := range values {
			sb.WriteString(fmt.Sprintf("  %s: %s\n",
				DetailValueStyle.Render(key),
				lipgloss.NewStyle().Foreground(ColorMuted).Render(v)))
		}
	}

	// Response headers
	sb.WriteString("\n" + DetailLabelStyle.Render("Response Headers:\n"))
	for key, values := range req.Response.Headers {
		for _, v := range values {
			sb.WriteString(fmt.Sprintf("  %s: %s\n",
				DetailValueStyle.Render(key),
				lipgloss.NewStyle().Foreground(ColorMuted).Render(v)))
		}
	}

	// Body preview (if available)
	if req.Response.Raw != "" {
		sb.WriteString("\n" + DetailLabelStyle.Render("Response Body:\n"))
		contentType := ""
		if ct, ok := req.Response.Headers["Content-Type"]; ok && len(ct) > 0 {
			contentType = ct[0]
		}
		body := util.FormatBody(req.Response.Raw, contentType)
		sb.WriteString(body)
	}

	return sb.String()
}

// renderFooter renders the help footer
func (a *App) renderFooter() string {
	var help string
	if a.focus == FocusDetail {
		help = fmt.Sprintf("%s scroll  %s back  %s replay  %s quit",
			HelpKeyStyle.Render("j/k"),
			HelpKeyStyle.Render("esc"),
			HelpKeyStyle.Render("r"),
			HelpKeyStyle.Render("q"))
	} else {
		help = fmt.Sprintf("%s navigate  %s expand  %s replay  %s quit",
			HelpKeyStyle.Render("j/k"),
			HelpKeyStyle.Render("enter"),
			HelpKeyStyle.Render("r"),
			HelpKeyStyle.Render("q"))
	}

	// Add error message if present
	if a.lastError != nil {
		errMsg := ErrorStyle.Render(fmt.Sprintf(" Error: %s", a.lastError.Error()))
		help = errMsg + "  " + help
	}

	return HelpStyle.Width(a.width).Padding(0, 1).Render(help)
}

// updateViewportSize updates the viewport dimensions
func (a *App) updateViewportSize() {
	contentHeight := a.height - 6 // header + footer + borders
	a.viewport = viewport.New(a.width-4, contentHeight)
	a.viewport.Style = lipgloss.NewStyle()
}

// updateDetailContent updates the viewport content with current selection
func (a *App) updateDetailContent() {
	if len(a.requests) == 0 || a.selected >= len(a.requests) {
		return
	}
	req := a.requests[a.selected]
	content := a.renderRequestDetail(req, a.width-6, a.height-6, true)
	a.viewport.SetContent(content)
	a.viewport.GotoTop()
}

// Command helpers

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return messages.TickMsg{Time: t}
	})
}

func (a *App) fetchTunnels() tea.Cmd {
	return func() tea.Msg {
		tunnels, err := a.client.GetTunnels()
		return messages.TunnelsMsg{Tunnels: tunnels, Err: err}
	}
}

func (a *App) fetchRequests() tea.Cmd {
	return func() tea.Msg {
		requests, err := a.client.GetRequests(50)
		return messages.RequestsMsg{Requests: requests, Err: err}
	}
}

func (a *App) replayRequest(id string) tea.Cmd {
	return func() tea.Msg {
		err := a.client.Replay(id)
		return messages.ReplayMsg{RequestID: id, Err: err}
	}
}

// Utility functions
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
