package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
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
	FocusDetailPanel // Detail panel in split view (scrollable)
	FocusSearch      // Search input mode
	FocusFilter      // Filter mode
)

// FilterStep represents the current step in filter creation
type FilterStep int

const (
	FilterStepField FilterStep = iota
	FilterStepOperator
	FilterStepValue
)

// Filter represents an active filter
type Filter struct {
	Field    string
	Operator string
	Value    string
}

// FilterField defines a filterable field
type FilterField struct {
	Name      string
	Key       string
	Operators []string
}

var filterFields = []FilterField{
	{Name: "Status Code", Key: "status", Operators: []string{"=", "!=", ">", "<", ">=", "<="}},
	{Name: "Method", Key: "method", Operators: []string{"=", "!="}},
	{Name: "Path", Key: "path", Operators: []string{"contains", "=", "starts", "ends"}},
	{Name: "Duration (ms)", Key: "duration", Operators: []string{"=", ">", "<", ">=", "<="}},
}

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
	focus     FocusState
	prevFocus FocusState // To restore after search/filter

	// Data
	tunnels        []ngrok.Tunnel
	requests       []ngrok.Request
	filteredReqs   []ngrok.Request // Filtered requests for display
	selected       int
	lastError      error
	lastSelectedID string // Track selected request ID for viewport updates
	
	// Status messages
	statusMessage     string
	statusMessageTime time.Time

	// Search (full-text with highlighting)
	searchQuery  string
	searchCursor int

	// Filter (field-based conditions)
	filterStep     FilterStep
	filterInput    string
	filterCursor   int
	filterSelected int              // Selected item in field/operator list
	activeFilters  []Filter         // Currently active filters
	pendingFilter  Filter           // Filter being created
	filteredFields []FilterField    // Filtered field list based on input

	// Components
	detailViewport viewport.Model // For detail panel scrolling
	spinner        spinner.Model
	keys           KeyMap

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

	case tea.MouseMsg:
		// Handle mouse wheel scrolling on detail panel regardless of focus
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				a.detailViewport.LineUp(3)
			case tea.MouseButtonWheelDown:
				a.detailViewport.LineDown(3)
			}
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.updateViewportSize()
		a.ready = true
		// Force update of detail viewport on resize
		a.lastSelectedID = ""
		a.updateDetailViewport()

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
			// Apply current filters
			a.applyFilters()
			if a.selected >= len(a.filteredReqs) {
				a.selected = max(0, len(a.filteredReqs)-1)
			}
			// Update detail view if we have new data
			if len(a.requests) != oldLen {
				a.updateDetailViewport()
			}
			a.lastError = nil
		}

	case messages.CopyMsg:
		if msg.Success {
			a.lastError = nil
			a.statusMessage = "Copied!"
			a.statusMessageTime = time.Now()
		}

	case messages.ErrorMsg:
		a.lastError = msg.Err

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

	// Update detail viewport when focused
	if a.focus == FocusDetailPanel {
		var cmd tea.Cmd
		a.detailViewport, cmd = a.detailViewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}

// handleKeyPress processes key events
func (a *App) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	// Handle search mode input
	if a.focus == FocusSearch {
		return a.handleSearchInput(msg)
	}

	// Handle filter mode input
	if a.focus == FocusFilter {
		return a.handleFilterInput(msg)
	}

	switch {
	case key.Matches(msg, a.keys.Quit):
		return tea.Quit

	case key.Matches(msg, a.keys.Search):
		a.prevFocus = a.focus
		a.focus = FocusSearch
		a.searchCursor = len(a.searchQuery)
		return nil

	case key.Matches(msg, a.keys.Filter):
		a.prevFocus = a.focus
		a.focus = FocusFilter
		a.filterStep = FilterStepField
		a.filterInput = ""
		a.filterCursor = 0
		a.filterSelected = 0
		a.filteredFields = filterFields
		return nil

	case key.Matches(msg, a.keys.Clear):
		a.clearAll()
		return nil

	case key.Matches(msg, a.keys.Copy):
		if len(a.filteredReqs) > 0 && a.selected < len(a.filteredReqs) {
			return a.copyAsCurl(a.filteredReqs[a.selected])
		}

	case key.Matches(msg, a.keys.Down):
		if a.focus == FocusList {
			if len(a.filteredReqs) > 0 {
				a.selected = min(a.selected+1, len(a.filteredReqs)-1)
				a.updateDetailViewport()
			}
		} else if a.focus == FocusDetailPanel {
			a.detailViewport.LineDown(1)
		}

	case key.Matches(msg, a.keys.Up):
		if a.focus == FocusList {
			if len(a.filteredReqs) > 0 {
				a.selected = max(a.selected-1, 0)
				a.updateDetailViewport()
			}
		} else if a.focus == FocusDetailPanel {
			a.detailViewport.LineUp(1)
		}

	case key.Matches(msg, a.keys.Top):
		if a.focus == FocusList {
			a.selected = 0
			a.updateDetailViewport()
		} else if a.focus == FocusDetailPanel {
			a.detailViewport.GotoTop()
		}

	case key.Matches(msg, a.keys.Bottom):
		if a.focus == FocusList && len(a.filteredReqs) > 0 {
			a.selected = len(a.filteredReqs) - 1
			a.updateDetailViewport()
		} else if a.focus == FocusDetailPanel {
			a.detailViewport.GotoBottom()
		}

	case key.Matches(msg, a.keys.Escape):
		if a.searchQuery != "" || len(a.activeFilters) > 0 {
			a.clearAll()
		} else if a.focus == FocusDetailPanel {
			a.focus = FocusList
		}

	case key.Matches(msg, a.keys.Toggle):
		if a.focus == FocusList {
			a.focus = FocusDetailPanel
		} else {
			a.focus = FocusList
		}

	case key.Matches(msg, a.keys.Replay):
		if len(a.filteredReqs) > 0 && a.selected < len(a.filteredReqs) {
			return a.replayRequest(a.filteredReqs[a.selected].ID)
		}
	}

	return nil
}

// handleSearchInput handles keyboard input in search mode
func (a *App) handleSearchInput(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEnter:
		a.focus = a.prevFocus
		a.performSearch()
		return nil

	case tea.KeyEscape:
		a.focus = a.prevFocus
		// Clear search completely
		a.searchQuery = ""
		a.searchCursor = 0
		// Force re-render of detail panel to remove highlighting
		a.lastSelectedID = ""
		// Reset to show all requests (respecting active filters)
		a.applyFilters()
		return nil

	case tea.KeyBackspace:
		if len(a.searchQuery) > 0 && a.searchCursor > 0 {
			a.searchQuery = a.searchQuery[:a.searchCursor-1] + a.searchQuery[a.searchCursor:]
			a.searchCursor--
			// Force re-render of detail panel for live highlighting
			a.lastSelectedID = ""
			a.updateDetailViewport()
		}
		return nil

	case tea.KeyLeft:
		if a.searchCursor > 0 {
			a.searchCursor--
		}
		return nil

	case tea.KeyRight:
		if a.searchCursor < len(a.searchQuery) {
			a.searchCursor++
		}
		return nil

	case tea.KeyRunes:
		char := string(msg.Runes)
		a.searchQuery = a.searchQuery[:a.searchCursor] + char + a.searchQuery[a.searchCursor:]
		a.searchCursor += len(char)
		// Force re-render of detail panel for live highlighting
		a.lastSelectedID = ""
		a.updateDetailViewport()
		return nil
	}
	return nil
}

// handleFilterInput handles keyboard input in filter mode
func (a *App) handleFilterInput(msg tea.KeyMsg) tea.Cmd {
	switch a.filterStep {
	case FilterStepField:
		return a.handleFilterFieldInput(msg)
	case FilterStepOperator:
		return a.handleFilterOperatorInput(msg)
	case FilterStepValue:
		return a.handleFilterValueInput(msg)
	}
	return nil
}

func (a *App) handleFilterFieldInput(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEscape:
		a.focus = a.prevFocus
		a.filterInput = ""
		return nil

	case tea.KeyEnter:
		if len(a.filteredFields) > 0 && a.filterSelected < len(a.filteredFields) {
			a.pendingFilter.Field = a.filteredFields[a.filterSelected].Key
			a.filterStep = FilterStepOperator
			a.filterSelected = 0
			a.filterInput = ""
		}
		return nil

	case tea.KeyUp:
		if a.filterSelected > 0 {
			a.filterSelected--
		}
		return nil

	case tea.KeyDown:
		if a.filterSelected < len(a.filteredFields)-1 {
			a.filterSelected++
		}
		return nil

	case tea.KeyBackspace:
		if len(a.filterInput) > 0 {
			a.filterInput = a.filterInput[:len(a.filterInput)-1]
			a.updateFilteredFields()
		}
		return nil

	case tea.KeyRunes:
		a.filterInput += string(msg.Runes)
		a.updateFilteredFields()
		a.filterSelected = 0
		return nil
	}
	return nil
}

func (a *App) handleFilterOperatorInput(msg tea.KeyMsg) tea.Cmd {
	field := a.getFieldByKey(a.pendingFilter.Field)
	if field == nil {
		return nil
	}

	switch msg.Type {
	case tea.KeyEscape:
		a.filterStep = FilterStepField
		a.filterSelected = 0
		a.updateFilteredFields()
		return nil

	case tea.KeyEnter:
		if a.filterSelected < len(field.Operators) {
			a.pendingFilter.Operator = field.Operators[a.filterSelected]
			a.filterStep = FilterStepValue
			a.filterInput = ""
			a.filterCursor = 0
		}
		return nil

	case tea.KeyUp:
		if a.filterSelected > 0 {
			a.filterSelected--
		}
		return nil

	case tea.KeyDown:
		if a.filterSelected < len(field.Operators)-1 {
			a.filterSelected++
		}
		return nil
	}
	return nil
}

func (a *App) handleFilterValueInput(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEscape:
		a.filterStep = FilterStepOperator
		a.filterSelected = 0
		return nil

	case tea.KeyEnter:
		if a.filterInput != "" {
			a.pendingFilter.Value = a.filterInput
			a.activeFilters = append(a.activeFilters, a.pendingFilter)
			a.pendingFilter = Filter{}
			a.filterInput = ""
			a.focus = a.prevFocus
			a.applyFilters()
		}
		return nil

	case tea.KeyBackspace:
		if len(a.filterInput) > 0 && a.filterCursor > 0 {
			a.filterInput = a.filterInput[:a.filterCursor-1] + a.filterInput[a.filterCursor:]
			a.filterCursor--
		}
		return nil

	case tea.KeyLeft:
		if a.filterCursor > 0 {
			a.filterCursor--
		}
		return nil

	case tea.KeyRight:
		if a.filterCursor < len(a.filterInput) {
			a.filterCursor++
		}
		return nil

	case tea.KeyRunes:
		char := string(msg.Runes)
		a.filterInput = a.filterInput[:a.filterCursor] + char + a.filterInput[a.filterCursor:]
		a.filterCursor += len(char)
		return nil
	}
	return nil
}

func (a *App) updateFilteredFields() {
	if a.filterInput == "" {
		a.filteredFields = filterFields
		return
	}
	query := strings.ToLower(a.filterInput)
	a.filteredFields = nil
	for _, f := range filterFields {
		if strings.Contains(strings.ToLower(f.Name), query) ||
			strings.Contains(strings.ToLower(f.Key), query) {
			a.filteredFields = append(a.filteredFields, f)
		}
	}
}

func (a *App) getFieldByKey(key string) *FilterField {
	for _, f := range filterFields {
		if f.Key == key {
			return &f
		}
	}
	return nil
}

// performSearch applies search and resets selection
func (a *App) performSearch() {
	a.selected = 0
	// Force re-render of detail panel by clearing lastSelectedID
	a.lastSelectedID = ""
	a.applyFilters()
}

// applyFilters applies all active filters AND search query to requests
func (a *App) applyFilters() {
	// Remember current selection by ID if possible
	var selectedID string
	if len(a.filteredReqs) > 0 && a.selected < len(a.filteredReqs) {
		selectedID = a.filteredReqs[a.selected].ID
	}

	// Start with all requests
	baseReqs := a.requests

	// Apply active filters first
	if len(a.activeFilters) > 0 {
		var filtered []ngrok.Request
		for _, req := range baseReqs {
			if a.matchesAllFilters(req) {
				filtered = append(filtered, req)
			}
		}
		baseReqs = filtered
	}

	// Then apply search query if present
	if a.searchQuery != "" {
		query := strings.ToLower(a.searchQuery)
		var filtered []ngrok.Request
		for _, req := range baseReqs {
			if a.matchesSearch(req, query) {
				filtered = append(filtered, req)
			}
		}
		a.filteredReqs = filtered
	} else {
		a.filteredReqs = baseReqs
	}

	// Try to restore selection by ID
	if selectedID != "" {
		for i, req := range a.filteredReqs {
			if req.ID == selectedID {
				a.selected = i
				a.updateDetailViewport()
				return
			}
		}
	}

	// If not found, clamp selection
	if a.selected >= len(a.filteredReqs) {
		a.selected = max(0, len(a.filteredReqs)-1)
	}
	a.updateDetailViewport()
}

// matchesSearch checks if a request matches the search query
func (a *App) matchesSearch(req ngrok.Request, query string) bool {
	// Search in method
	if strings.Contains(strings.ToLower(req.Request.Method), query) {
		return true
	}
	// Search in path
	if strings.Contains(strings.ToLower(req.Request.URI), query) {
		return true
	}
	// Search in status
	if strings.Contains(fmt.Sprintf("%d", req.StatusCode()), query) {
		return true
	}
	// Search in headers
	for k, vals := range req.Request.Headers {
		for _, v := range vals {
			if strings.Contains(strings.ToLower(k+": "+v), query) {
				return true
			}
		}
	}
	for k, vals := range req.Response.Headers {
		for _, v := range vals {
			if strings.Contains(strings.ToLower(k+": "+v), query) {
				return true
			}
		}
	}
	// Search in body
	reqBody := req.Request.DecodeBody()
	if strings.Contains(strings.ToLower(reqBody), query) {
		return true
	}
	respBody := req.Response.DecodeBody()
	if strings.Contains(strings.ToLower(respBody), query) {
		return true
	}
	return false
}

// matchesAllFilters checks if a request matches all active filters
func (a *App) matchesAllFilters(req ngrok.Request) bool {
	for _, f := range a.activeFilters {
		if !a.matchesFilter(req, f) {
			return false
		}
	}
	return true
}

// matchesFilter checks if a request matches a single filter
func (a *App) matchesFilter(req ngrok.Request, f Filter) bool {
	switch f.Field {
	case "status":
		return a.compareNumeric(req.StatusCode(), f.Operator, f.Value)
	case "method":
		return a.compareString(req.Request.Method, f.Operator, f.Value)
	case "path":
		return a.compareString(req.Request.URI, f.Operator, f.Value)
	case "duration":
		return a.compareFloat(req.DurationMs(), f.Operator, f.Value)
	}
	return true
}

func (a *App) compareNumeric(val int, op string, target string) bool {
	t, err := strconv.Atoi(target)
	if err != nil {
		return false
	}
	switch op {
	case "=":
		return val == t
	case "!=":
		return val != t
	case ">":
		return val > t
	case "<":
		return val < t
	case ">=":
		return val >= t
	case "<=":
		return val <= t
	}
	return false
}

func (a *App) compareFloat(val float64, op string, target string) bool {
	t, err := strconv.ParseFloat(target, 64)
	if err != nil {
		return false
	}
	switch op {
	case "=":
		return val == t
	case ">":
		return val > t
	case "<":
		return val < t
	case ">=":
		return val >= t
	case "<=":
		return val <= t
	}
	return false
}

func (a *App) compareString(val string, op string, target string) bool {
	val = strings.ToLower(val)
	target = strings.ToLower(target)
	switch op {
	case "=":
		return val == target
	case "!=":
		return val != target
	case "contains":
		return strings.Contains(val, target)
	case "starts":
		return strings.HasPrefix(val, target)
	case "ends":
		return strings.HasSuffix(val, target)
	}
	return false
}

// clearAll clears search and all filters
func (a *App) clearAll() {
	a.searchQuery = ""
	a.searchCursor = 0
	a.activeFilters = nil
	a.filteredReqs = a.requests
	a.selected = 0
	// Force re-render to remove highlighting
	a.lastSelectedID = ""
	a.updateDetailViewport()
}

// copyAsCurl copies the request as a cURL command to clipboard
func (a *App) copyAsCurl(req ngrok.Request) tea.Cmd {
	// Get the base URL from tunnels
	baseURL := ""
	if len(a.tunnels) > 0 {
		baseURL = a.tunnels[0].PublicURL
	}
	
	return func() tea.Msg {
		curl := buildCurlCommand(req, baseURL)
		
		// Try to copy to clipboard using system command
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("pbcopy")
		case "linux":
			cmd = exec.Command("xclip", "-selection", "clipboard")
		default:
			return messages.ErrorMsg{Err: fmt.Errorf("clipboard not supported on %s", runtime.GOOS)}
		}

		cmd.Stdin = strings.NewReader(curl)
		if err := cmd.Run(); err != nil {
			return messages.ErrorMsg{Err: fmt.Errorf("failed to copy: %w", err)}
		}

		return messages.CopyMsg{Success: true}
	}
}

// buildCurlCommand builds a cURL command string from a request
func buildCurlCommand(req ngrok.Request, baseURL string) string {
	var parts []string
	parts = append(parts, "curl")

	// Method
	if req.Request.Method != "GET" {
		parts = append(parts, "-X", req.Request.Method)
	}

	// Headers (skip internal/automatic headers)
	for key, values := range req.Request.Headers {
		lowerKey := strings.ToLower(key)
		// Skip headers that curl handles automatically or are ngrok-specific
		if lowerKey == "host" ||
			lowerKey == "content-length" ||
			lowerKey == "accept-encoding" ||
			lowerKey == "user-agent" ||
			strings.HasPrefix(lowerKey, "x-forwarded") {
			continue
		}
		for _, v := range values {
			parts = append(parts, "-H", fmt.Sprintf("'%s: %s'", key, v))
		}
	}

	// Body
	body := req.Request.DecodeBody()
	if body != "" {
		// Escape single quotes in body
		body = strings.ReplaceAll(body, "'", "'\\''")
		parts = append(parts, "-d", fmt.Sprintf("'%s'", body))
	}

	// Full URL
	fullURL := baseURL + req.Request.URI
	parts = append(parts, fmt.Sprintf("'%s'", fullURL))

	return strings.Join(parts, " ")
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

	// Responsive layout
	if a.width >= 120 {
		return a.renderSideBySide(contentHeight)
	}
	return a.renderStacked(contentHeight)
}

// renderSideBySide renders list and detail side by side
func (a *App) renderSideBySide(height int) string {
	// Give 30% to list, 70% to detail for more space to view request/response
	listWidth := a.width * 30 / 100
	if listWidth < 36 {
		listWidth = 36
	}
	detailWidth := a.width - listWidth

	// Content dimensions (subtract border=2 + padding=2 = 4)
	listContentWidth := listWidth - 4
	detailContentWidth := detailWidth - 4
	contentHeight := height - 2 // border top + bottom

	list := a.renderRequestList(listContentWidth, contentHeight)
	detail := a.renderDetailPanel(detailContentWidth, contentHeight)

	// Highlight focused panel
	listBorder := BorderStyle
	detailBorder := BorderStyle
	if a.focus == FocusList || a.focus == FocusFilter {
		listBorder = ActiveBorderStyle
	} else if a.focus == FocusDetailPanel {
		detailBorder = ActiveBorderStyle
	}

	listBox := listBorder.Width(listContentWidth).Height(contentHeight).Render(list)
	detailBox := detailBorder.Width(detailContentWidth).Height(contentHeight).Render(detail)

	return lipgloss.JoinHorizontal(lipgloss.Top, listBox, detailBox)
}

// renderStacked renders list above detail
func (a *App) renderStacked(height int) string {
	// Give 40% to list, 60% to detail
	listHeight := height * 40 / 100
	if listHeight < 8 {
		listHeight = 8
	}
	detailHeight := height - listHeight

	// Content dimensions (subtract border=2 + padding=2 = 4)
	contentWidth := a.width - 4
	listContentHeight := listHeight - 2
	detailContentHeight := detailHeight - 2

	list := a.renderRequestList(contentWidth, listContentHeight)
	detail := a.renderDetailPanel(contentWidth, detailContentHeight)

	// Highlight focused panel
	listBorder := BorderStyle
	detailBorder := BorderStyle
	if a.focus == FocusList || a.focus == FocusFilter {
		listBorder = ActiveBorderStyle
	} else if a.focus == FocusDetailPanel {
		detailBorder = ActiveBorderStyle
	}

	listBox := listBorder.Width(contentWidth).Height(listContentHeight).Render(list)
	detailBox := detailBorder.Width(contentWidth).Height(detailContentHeight).Render(detail)

	return lipgloss.JoinVertical(lipgloss.Left, listBox, detailBox)
}

// renderRequestList renders the list of requests
func (a *App) renderRequestList(width, height int) string {
	// If in filter mode, show filter UI at top
	if a.focus == FocusFilter {
		return a.renderFilterInPanel(width, height)
	}

	if len(a.requests) == 0 {
		msg := "Waiting for requests..."
		if a.loading {
			msg = a.spinner.View() + " " + msg
		}
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, msg)
	}

	if len(a.filteredReqs) == 0 && (len(a.activeFilters) > 0 || a.searchQuery != "") {
		msg := "No matching requests"
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, msg)
	}

	var lines []string

	// Title with filter/search count
	title := ListTitleStyle.Render("Requests")
	if len(a.activeFilters) > 0 || a.searchQuery != "" {
		filterInfo := lipgloss.NewStyle().Foreground(ColorMuted).
			Render(fmt.Sprintf(" (%d/%d)", len(a.filteredReqs), len(a.requests)))
		title = title + filterInfo
	}
	lines = append(lines, title)

	visibleLines := height - 2

	// Calculate scroll offset
	startIdx := 0
	if a.selected >= visibleLines {
		startIdx = a.selected - visibleLines + 1
	}

	endIdx := min(startIdx+visibleLines, len(a.filteredReqs))

	for i := startIdx; i < endIdx; i++ {
		req := a.filteredReqs[i]
		line := a.renderRequestLine(req, width-2, i == a.selected)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// renderFilterInPanel renders the filter UI inside the request list panel
func (a *App) renderFilterInPanel(width, height int) string {
	var lines []string

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	mutedStyle := lipgloss.NewStyle().Foreground(ColorMuted)
	selectedStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)

	switch a.filterStep {
	case FilterStepField:
		lines = append(lines, titleStyle.Render("Add Filter"))
		lines = append(lines, "")

		if a.filterInput != "" {
			lines = append(lines, mutedStyle.Render("Search: ")+a.filterInput+"█")
			lines = append(lines, "")
		}

		for i, f := range a.filteredFields {
			if i == a.filterSelected {
				lines = append(lines, selectedStyle.Render("▶ "+f.Name))
			} else {
				lines = append(lines, "  "+f.Name)
			}
		}

		if len(a.filteredFields) == 0 {
			lines = append(lines, mutedStyle.Render("  No matching fields"))
		}

	case FilterStepOperator:
		field := a.getFieldByKey(a.pendingFilter.Field)
		if field != nil {
			lines = append(lines, titleStyle.Render("Select Operator"))
			lines = append(lines, mutedStyle.Render("Field: "+field.Name))
			lines = append(lines, "")

			for i, op := range field.Operators {
				if i == a.filterSelected {
					lines = append(lines, selectedStyle.Render("▶ "+op))
				} else {
					lines = append(lines, "  "+op)
				}
			}
		}

	case FilterStepValue:
		field := a.getFieldByKey(a.pendingFilter.Field)
		if field != nil {
			lines = append(lines, titleStyle.Render("Enter Value"))
			lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s %s ?", field.Name, a.pendingFilter.Operator)))
			lines = append(lines, "")
			lines = append(lines, "> "+a.filterInput+"█")
		}
	}

	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("↑↓: select  Enter: confirm  Esc: cancel"))

	return strings.Join(lines, "\n")
}

// renderRequestLine renders a single request line (compact mode)
func (a *App) renderRequestLine(req ngrok.Request, width int, selected bool) string {
	statusCode := req.StatusCode()
	timeAgo := formatRelativeTime(req.Start)

	// Compact format: "▶ METHOD  STATUS PATH         TIME"
	// Widths:          2  8       4     var          6
	// METHOD is 8 chars to fit "OPTIONS" (7) + space
	fixedWidth := 2 + 8 + 4 + 6
	pathWidth := width - fixedWidth
	if pathWidth < 8 {
		pathWidth = 8
	}

	pathStr := util.TruncateString(req.Request.URI, pathWidth)

	// Build the line with proper formatting
	var indicator string
	if selected {
		indicator = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("▶ ")
	} else {
		indicator = "  "
	}

	// Apply highlighting if search is active
	methodStr := req.Request.Method
	statusStr := fmt.Sprintf("%d", statusCode)

	if a.searchQuery != "" {
		methodStr = a.highlightText(methodStr)
		statusStr = a.highlightText(statusStr)
		pathStr = a.highlightText(pathStr)
	}

	method := lipgloss.NewStyle().
		Bold(true).
		Foreground(MethodColor(req.Request.Method)).
		Width(8).
		Render(methodStr)

	status := lipgloss.NewStyle().
		Bold(true).
		Foreground(StatusCodeColor(statusCode)).
		Width(4).
		Render(statusStr)

	path := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#D1D5DB")).
		Width(pathWidth).
		Render(pathStr)

	time := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Width(6).
		Align(lipgloss.Right).
		Render(timeAgo)

	return fmt.Sprintf("%s%s%s%s%s", indicator, method, status, path, time)
}

// highlightText highlights search query matches in text with yellow background
func (a *App) highlightText(text string) string {
	if a.searchQuery == "" {
		return text
	}

	query := strings.ToLower(a.searchQuery)
	lowerText := strings.ToLower(text)

	// Find all match positions
	var result strings.Builder
	lastEnd := 0

	highlightStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#FBBF24")).
		Foreground(lipgloss.Color("#000000"))

	for {
		idx := strings.Index(lowerText[lastEnd:], query)
		if idx == -1 {
			result.WriteString(text[lastEnd:])
			break
		}

		matchStart := lastEnd + idx
		matchEnd := matchStart + len(a.searchQuery)

		// Add text before match
		result.WriteString(text[lastEnd:matchStart])
		// Add highlighted match (preserve original case)
		result.WriteString(highlightStyle.Render(text[matchStart:matchEnd]))

		lastEnd = matchEnd
	}

	return result.String()
}

// formatRelativeTime formats a time as relative (e.g., "2m ago", "5s ago")
func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	diff := time.Since(t)

	switch {
	case diff < time.Second:
		return "now"
	case diff < time.Minute:
		return fmt.Sprintf("%ds", int(diff.Seconds()))
	case diff < time.Hour:
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh", int(diff.Hours()))
	default:
		return fmt.Sprintf("%dd", int(diff.Hours()/24))
	}
}

// httpStatusText returns the standard HTTP status text for a status code
func httpStatusText(code int) string {
	statusTexts := map[int]string{
		100: "Continue",
		101: "Switching Protocols",
		200: "OK",
		201: "Created",
		202: "Accepted",
		204: "No Content",
		206: "Partial Content",
		301: "Moved Permanently",
		302: "Found",
		303: "See Other",
		304: "Not Modified",
		307: "Temporary Redirect",
		308: "Permanent Redirect",
		400: "Bad Request",
		401: "Unauthorized",
		403: "Forbidden",
		404: "Not Found",
		405: "Method Not Allowed",
		406: "Not Acceptable",
		408: "Request Timeout",
		409: "Conflict",
		410: "Gone",
		411: "Length Required",
		412: "Precondition Failed",
		413: "Payload Too Large",
		414: "URI Too Long",
		415: "Unsupported Media Type",
		416: "Range Not Satisfiable",
		422: "Unprocessable Entity",
		429: "Too Many Requests",
		500: "Internal Server Error",
		501: "Not Implemented",
		502: "Bad Gateway",
		503: "Service Unavailable",
		504: "Gateway Timeout",
	}
	if text, ok := statusTexts[code]; ok {
		return text
	}
	return ""
}

// renderDetailPanel renders the detail panel (side panel mode)
func (a *App) renderDetailPanel(width, height int) string {
	if len(a.filteredReqs) == 0 || a.selected >= len(a.filteredReqs) {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
			"Select a request to view details")
	}

	// Use viewport for scrollable content
	return a.detailViewport.View()
}

// renderRequestDetail renders request details
func (a *App) renderRequestDetail(req ngrok.Request, width, height int, full bool) string {
	var sb strings.Builder

	// Title with colored method (badge style)
	method := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#000000")).
		Background(MethodColor(req.Request.Method)).
		Padding(0, 1).
		Render(req.Request.Method)

	// Highlight endpoint if search is active
	endpointText := req.Request.URI
	if a.searchQuery != "" {
		endpointText = a.highlightText(endpointText)
	}
	endpoint := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Render(endpointText)
	sb.WriteString(fmt.Sprintf("%s %s\n\n", method, endpoint))

	// Status with full text
	statusCode := req.StatusCode()
	statusText := fmt.Sprintf("%d %s", statusCode, httpStatusText(statusCode))
	if a.searchQuery != "" {
		statusText = a.highlightText(statusText)
	}
	status := StatusStyle.Foreground(StatusCodeColor(statusCode)).Render(statusText)
	sb.WriteString(fmt.Sprintf("Status:   %s\n", status))

	// Duration
	sb.WriteString(fmt.Sprintf("Duration: %.2fms\n", req.DurationMs()))

	// Timestamp
	timestamp := req.Start.Format("2006-01-02 15:04:05")
	sb.WriteString(fmt.Sprintf("Time:     %s\n", timestamp))

	// Request headers (sorted to prevent flickering)
	sb.WriteString("\n")
	sb.WriteString(DetailLabelStyle.Render("Request Headers:"))
	sb.WriteString("\n")
	sb.WriteString(a.renderHeaders(req.Request.Headers))

	// Request body (if available) - decode from base64
	reqBody := req.Request.DecodeBody()
	if reqBody != "" {
		sb.WriteString("\n")
		sb.WriteString(DetailLabelStyle.Render("Request Body:"))
		sb.WriteString("\n")
		reqContentType := ""
		if ct, ok := req.Request.Headers["Content-Type"]; ok && len(ct) > 0 {
			reqContentType = ct[0]
		}
		formattedReqBody := util.FormatBody(reqBody, reqContentType)
		if a.searchQuery != "" {
			formattedReqBody = a.highlightText(formattedReqBody)
		}
		sb.WriteString(indentLines(formattedReqBody, "  "))
		sb.WriteString("\n") // Extra blank line after request body
	}

	// Response headers (sorted to prevent flickering)
	sb.WriteString("\n")
	sb.WriteString(DetailLabelStyle.Render("Response Headers:"))
	sb.WriteString("\n")
	sb.WriteString(a.renderHeaders(req.Response.Headers))

	// Response body (if available) - decode from base64
	respBody := req.Response.DecodeBody()
	if respBody != "" {
		sb.WriteString("\n")
		sb.WriteString(DetailLabelStyle.Render("Response Body:"))
		sb.WriteString("\n")
		respContentType := ""
		if ct, ok := req.Response.Headers["Content-Type"]; ok && len(ct) > 0 {
			respContentType = ct[0]
		}
		formattedRespBody := util.FormatBody(respBody, respContentType)
		if a.searchQuery != "" {
			formattedRespBody = a.highlightText(formattedRespBody)
		}
		sb.WriteString(indentLines(formattedRespBody, "  "))
	}

	return sb.String()
}

// indentLines adds a prefix to each line of text
func indentLines(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}



// renderHeaders renders headers in sorted order to prevent flickering
func (a *App) renderHeaders(headers map[string][]string) string {
	if len(headers) == 0 {
		return "  (none)\n"
	}

	// Get sorted keys
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, key := range keys {
		values := headers[key]
		for _, v := range values {
			headerLine := fmt.Sprintf("%s: %s", key, v)
			if a.searchQuery != "" {
				headerLine = a.highlightText(headerLine)
			}
			sb.WriteString(fmt.Sprintf("  %s\n", headerLine))
		}
	}
	return sb.String()
}

// renderFooter renders the help footer
func (a *App) renderFooter() string {
	// Search mode: show search input
	if a.focus == FocusSearch {
		prompt := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("/")

		// Build input with cursor
		input := a.searchQuery
		if a.searchCursor < len(input) {
			input = input[:a.searchCursor] + "█" + input[a.searchCursor:]
		} else {
			input = input + "█"
		}

		searchLine := fmt.Sprintf("%s %s", prompt, input)
		hint := lipgloss.NewStyle().Foreground(ColorMuted).
			Render("  (enter: search, esc: cancel)")

		return HelpStyle.Width(a.width).Padding(0, 1).Render(searchLine + hint)
	}


	// Build status line with active filters and search
	var statusParts []string

	// Show active filters
	for _, f := range a.activeFilters {
		badge := lipgloss.NewStyle().
			Background(ColorPrimary).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1).
			Render(fmt.Sprintf("%s %s %s", f.Field, f.Operator, f.Value))
		statusParts = append(statusParts, badge)
	}

	// Show search query
	if a.searchQuery != "" {
		searchBadge := lipgloss.NewStyle().
			Background(ColorWarning).
			Foreground(lipgloss.Color("#000000")).
			Padding(0, 1).
			Render("/" + a.searchQuery)
		statusParts = append(statusParts, searchBadge)
	}

	// Help text
	var help string
	if a.focus == FocusFilter {
		help = fmt.Sprintf("%s select  %s confirm  %s cancel",
			HelpKeyStyle.Render("↑↓"),
			HelpKeyStyle.Render("enter"),
			HelpKeyStyle.Render("esc"))
	} else if a.focus == FocusDetailPanel {
		help = fmt.Sprintf("%s scroll  %s list  %s copy  %s replay  %s quit",
			HelpKeyStyle.Render("j/k"),
			HelpKeyStyle.Render("tab"),
			HelpKeyStyle.Render("c"),
			HelpKeyStyle.Render("r"),
			HelpKeyStyle.Render("q"))
	} else {
		help = fmt.Sprintf("%s nav  %s search  %s filter  %s copy  %s replay  %s quit",
			HelpKeyStyle.Render("j/k"),
			HelpKeyStyle.Render("/"),
			HelpKeyStyle.Render("f"),
			HelpKeyStyle.Render("c"),
			HelpKeyStyle.Render("r"),
			HelpKeyStyle.Render("q"))
	}

	// Add clear hint if filters or search active
	if len(a.activeFilters) > 0 || a.searchQuery != "" {
		help = fmt.Sprintf("%s clear  ", HelpKeyStyle.Render("x")) + help
	}

	// Combine status and help
	var footer string
	if len(statusParts) > 0 {
		footer = strings.Join(statusParts, " ") + "  " + help
	} else {
		footer = help
	}

	// Add error message if present
	if a.lastError != nil {
		errMsg := ErrorStyle.Render(fmt.Sprintf("Error: %s  ", a.lastError.Error()))
		footer = errMsg + footer
	}

	// Add status message (e.g., "Copied!") - show for 1 second
	if a.statusMessage != "" && time.Since(a.statusMessageTime) < 1*time.Second {
		statusMsg := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#10B981")).
			Bold(true).
			Render(a.statusMessage + "  ")
		footer = statusMsg + footer
	} else if a.statusMessage != "" {
		// Clear expired message
		a.statusMessage = ""
	}

	return HelpStyle.Width(a.width).Padding(0, 1).Render(footer)
}

// updateViewportSize updates the viewport dimensions
func (a *App) updateViewportSize() {
	contentHeight := a.height - 4 // header + footer

	// Calculate detail panel size for split view
	var detailWidth, detailHeight int
	if a.width >= 120 {
		// Side by side: 30% list, 70% detail
		listWidth := a.width * 30 / 100
		if listWidth < 36 {
			listWidth = 36
		}
		detailWidth = a.width - listWidth - 4 // border + padding
		detailHeight = contentHeight - 2      // border
	} else {
		// Stacked: 40% list, 60% detail
		listHeight := contentHeight * 40 / 100
		if listHeight < 8 {
			listHeight = 8
		}
		detailWidth = a.width - 4               // border + padding
		detailHeight = contentHeight - listHeight - 2
	}
	a.detailViewport = viewport.New(detailWidth, detailHeight)
	a.detailViewport.Style = lipgloss.NewStyle()

	// Update content if we have requests
	a.updateDetailViewport()
}

// updateDetailViewport updates the split-view detail viewport
func (a *App) updateDetailViewport() {
	if len(a.filteredReqs) == 0 || a.selected >= len(a.filteredReqs) {
		a.detailViewport.SetContent("Select a request to view details")
		return
	}

	req := a.filteredReqs[a.selected]

	// Only update if selection changed
	if req.ID != a.lastSelectedID {
		a.lastSelectedID = req.ID
		content := a.renderRequestDetail(req, a.detailViewport.Width, a.detailViewport.Height, false)
		// Use lipgloss to wrap content to viewport width
		content = lipgloss.NewStyle().Width(a.detailViewport.Width).Render(content)
		a.detailViewport.SetContent(content)

		// If search is active, scroll to first match
		if a.searchQuery != "" {
			a.scrollToFirstMatch(req)
		} else {
			a.detailViewport.GotoTop()
		}
	}
}

// scrollToFirstMatch scrolls the detail viewport to the first occurrence of the search query
func (a *App) scrollToFirstMatch(req ngrok.Request) {
	query := strings.ToLower(a.searchQuery)

	// Build a simplified version of the content to find line numbers
	var lines []string
	lines = append(lines, req.Request.Method+" "+req.Request.URI) // Line 0-1: title
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("%d %s", req.StatusCode(), httpStatusText(req.StatusCode()))) // Status
	lines = append(lines, fmt.Sprintf("%.2fms", req.DurationMs()))                                  // Duration
	lines = append(lines, req.Start.Format("2006-01-02 15:04:05"))                                  // Time
	lines = append(lines, "")
	lines = append(lines, "Request Headers:")

	// Request headers
	for k, vals := range req.Request.Headers {
		for _, v := range vals {
			lines = append(lines, k+": "+v)
		}
	}

	// Request body
	reqBody := req.Request.DecodeBody()
	if reqBody != "" {
		lines = append(lines, "")
		lines = append(lines, "Request Body:")
		bodyLines := strings.Split(reqBody, "\n")
		lines = append(lines, bodyLines...)
	}

	lines = append(lines, "")
	lines = append(lines, "Response Headers:")

	// Response headers
	for k, vals := range req.Response.Headers {
		for _, v := range vals {
			lines = append(lines, k+": "+v)
		}
	}

	// Response body
	respBody := req.Response.DecodeBody()
	if respBody != "" {
		lines = append(lines, "")
		lines = append(lines, "Response Body:")
		bodyLines := strings.Split(respBody, "\n")
		lines = append(lines, bodyLines...)
	}

	// Find first line with match
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), query) {
			// Scroll to this line (with some padding above)
			targetLine := max(0, i-2)
			a.detailViewport.SetYOffset(targetLine)
			return
		}
	}

	// No match found in simplified content, go to top
	a.detailViewport.GotoTop()
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

func (a *App) replayRequest(requestID string) tea.Cmd {
	return func() tea.Msg {
		err := a.client.Replay(requestID)
		return messages.ReplayMsg{RequestID: requestID, Err: err}
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
