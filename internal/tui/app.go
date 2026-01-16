package tui

import (
	"fmt"
	"io"
	"net/http"
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

	"github.com/sung01299/mole/internal/ngrok"
	"github.com/sung01299/mole/internal/storage"
	"github.com/sung01299/mole/internal/tui/messages"
	"github.com/sung01299/mole/internal/util"
)

// FocusState represents which panel is currently focused
type FocusState int

const (
	FocusList        FocusState = iota
	FocusDetailPanel            // Detail panel in split view (scrollable)
	FocusSearch                 // Search input mode
	FocusFilter                 // Filter mode
	FocusReplayEdit             // Replay with edit mode
	FocusDiff                   // Diff view mode
	FocusHistory                // History view mode
)

// ReplayEditStep represents the current step in replay edit
type ReplayEditStep int

const (
	ReplayEditStepMain ReplayEditStep = iota // Main menu (Method, Path, Headers, Body, Send)
	ReplayEditStepMethod
	ReplayEditStepPath
	ReplayEditStepHeaders
	ReplayEditStepHeaderEdit // Editing a single header
	ReplayEditStepBody
)

// FilterStep represents the current step in filter creation
type FilterStep int

const (
	FilterStepField FilterStep = iota
	FilterStepOperator
	FilterStepUnit
	FilterStepValue
	FilterStepLogical // Ask for && or || after adding a filter
)

// FilterFieldType defines the type of filter field
type FilterFieldType int

const (
	FilterTypeString FilterFieldType = iota
	FilterTypeNumericWithUnit
)

// Filter represents an active filter
type Filter struct {
	Field           string
	Operator        string
	Unit            string // For numeric fields with units (ms, s, kb, etc.)
	Value           string
	LogicalOperator string // "&&" or "||" to chain with next filter
}

// HeaderEntry represents a header key-value pair for editing
type HeaderEntry struct {
	Key   string
	Value string
}

// HTTP methods for replay edit
var httpMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

// FilterField defines a filterable field
type FilterField struct {
	Name      string
	Key       string
	Type      FilterFieldType
	Operators []string
	Units     []string // For numeric fields
}

var filterFields = []FilterField{
	// Basic fields
	{Name: "Duration", Key: "duration", Type: FilterTypeNumericWithUnit, Operators: []string{">", "<", ">=", "<="}, Units: []string{"ms", "s", "m", "h", "d"}},
	{Name: "Path", Key: "path", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "ResponseSize", Key: "response_size", Type: FilterTypeNumericWithUnit, Operators: []string{">", "<", ">=", "<="}, Units: []string{"b", "kb", "mb"}},
	{Name: "StatusCode", Key: "status", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	// Headers
	{Name: "Headers.Accept", Key: "header.accept", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Accept-Charset", Key: "header.accept-charset", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Accept-Datetime", Key: "header.accept-datetime", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Accept-Encoding", Key: "header.accept-encoding", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Accept-Language", Key: "header.accept-language", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.A-IM", Key: "header.a-im", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Authorization", Key: "header.authorization", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Cache-Control", Key: "header.cache-control", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Connection", Key: "header.connection", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Content-Length", Key: "header.content-length", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Content-MD5", Key: "header.content-md5", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Content-Type", Key: "header.content-type", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Cookie", Key: "header.cookie", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Date", Key: "header.date", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Expect", Key: "header.expect", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.From", Key: "header.from", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Host", Key: "header.host", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Max-Forwards", Key: "header.max-forwards", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Origin", Key: "header.origin", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Pragma", Key: "header.pragma", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Proxy-Authorization", Key: "header.proxy-authorization", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Range", Key: "header.range", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Referer", Key: "header.referer", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.TE", Key: "header.te", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Upgrade", Key: "header.upgrade", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.User-Agent", Key: "header.user-agent", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Via", Key: "header.via", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Warning", Key: "header.warning", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.If-Match", Key: "header.if-match", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.If-Modified-Since", Key: "header.if-modified-since", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.If-None-Match", Key: "header.if-none-match", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.If-Range", Key: "header.if-range", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.If-Unmodified-Since", Key: "header.if-unmodified-since", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Forwarded", Key: "header.forwarded", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.X-Forwarded-For", Key: "header.x-forwarded-for", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.X-Forwarded-Host", Key: "header.x-forwarded-host", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.X-Forwarded-Proto", Key: "header.x-forwarded-proto", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Access-Control-Request-Headers", Key: "header.access-control-request-headers", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Access-Control-Request-Method", Key: "header.access-control-request-method", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
	{Name: "Headers.Server", Key: "header.server", Type: FilterTypeString, Operators: []string{"==", "!=", "match", "!match"}},
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
	filterSelected int           // Selected item in field/operator list
	activeFilters  []Filter      // Currently active filters
	pendingFilter  Filter        // Filter being created
	filteredFields []FilterField // Filtered field list based on input

	// Replay Edit
	replayEditStep     ReplayEditStep
	replayEditSelected int
	replayEditMethod   string
	replayEditPath     string
	replayEditHeaders  []HeaderEntry // Editable headers
	replayEditBody     string
	replayEditCursor   int    // Cursor position for text input
	replayEditInput    string // Current input text
	replayHeaderIdx    int    // Which header is being edited
	replayHeaderField  string // "key" or "value" being edited

	// Diff view
	diffRequestA   *ngrok.Request // First request for diff (nil if not selected)
	diffRequestB   *ngrok.Request // Second request for diff
	diffViewport   viewport.Model // Viewport for diff content
	diffScrollSync bool           // Whether to sync scroll between panels

	// History view
	historySessions     []storage.Session
	historySelectedSess int // Selected session index

	// Components
	detailViewport viewport.Model // For detail panel scrolling
	spinner        spinner.Model
	keys           KeyMap

	// API client
	client *ngrok.Client

	// Storage for persistent history
	storage          *storage.Storage
	savedReqIDs      map[string]bool // Track which requests have been saved
	viewingHistory   bool            // Whether we're viewing historical session
	viewingSessionID string          // ID of historical session being viewed

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

	// Initialize storage (non-fatal if it fails)
	store, err := storage.New()
	if err != nil {
		// Log error but continue without storage
		store = nil
	}

	return &App{
		client:      client,
		storage:     store,
		savedReqIDs: make(map[string]bool),
		keys:        DefaultKeyMap(),
		spinner:     s,
		loading:     true,
		windowFocus: true,
		focus:       FocusList,
	}
}

// Init implements tea.Model
func (a *App) Init() tea.Cmd {
	// Run cleanup on startup (keep 7 days or 1000 requests)
	if a.storage != nil {
		a.storage.Cleanup(7, 1000)
	}

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
		// Handle mouse wheel scrolling on detail/diff panel regardless of focus
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if a.focus == FocusDiff {
					a.diffViewport.LineUp(3)
				} else {
					a.detailViewport.LineUp(3)
				}
			case tea.MouseButtonWheelDown:
				if a.focus == FocusDiff {
					a.diffViewport.LineDown(3)
				} else {
					a.detailViewport.LineDown(3)
				}
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

			// Start storage session if we have tunnels and storage is available
			if a.storage != nil && len(a.tunnels) > 0 && a.storage.CurrentSessionID() == "" {
				tunnelURL := a.tunnels[0].PublicURL
				a.storage.StartSession(tunnelURL)
			}
		}

	case messages.RequestsMsg:
		a.loading = false
		if msg.Err != nil {
			a.lastError = msg.Err
		} else if !a.viewingHistory {
			// Only update if not viewing historical session
			// Preserve selection if possible
			oldLen := len(a.requests)
			a.requests = msg.Requests

			// Auto-save new requests to storage
			a.saveNewRequests()

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

	// Handle replay edit mode input
	if a.focus == FocusReplayEdit {
		return a.handleReplayEditInput(msg)
	}

	// Handle diff view input
	if a.focus == FocusDiff {
		return a.handleDiffInput(msg)
	}

	// Handle history view input
	if a.focus == FocusHistory {
		return a.handleHistoryInput(msg)
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
		if a.diffRequestA != nil {
			// Cancel diff selection
			a.diffRequestA = nil
		} else if a.searchQuery != "" || len(a.activeFilters) > 0 {
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

	case key.Matches(msg, a.keys.ReplayEdit):
		if len(a.filteredReqs) > 0 && a.selected < len(a.filteredReqs) {
			a.initReplayEdit(a.filteredReqs[a.selected])
			a.prevFocus = a.focus
			a.focus = FocusReplayEdit
		}

	case key.Matches(msg, a.keys.Diff):
		if len(a.filteredReqs) > 0 && a.selected < len(a.filteredReqs) {
			req := a.filteredReqs[a.selected]
			if a.diffRequestA == nil {
				// First request - mark it
				a.diffRequestA = &req
			} else if a.diffRequestA.ID == req.ID {
				// Same request - unmark
				a.diffRequestA = nil
			} else {
				// Second request - show diff
				a.diffRequestB = &req
				a.initDiffView()
				a.prevFocus = a.focus
				a.focus = FocusDiff
			}
		}

	case key.Matches(msg, a.keys.History):
		// If viewing history, go back to live
		if a.viewingHistory {
			a.exitHistoryView()
		} else {
			a.prevFocus = a.focus
			a.focus = FocusHistory
			a.initHistoryView()
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
	case FilterStepUnit:
		return a.handleFilterUnitInput(msg)
	case FilterStepValue:
		return a.handleFilterValueInput(msg)
	case FilterStepLogical:
		return a.handleFilterLogicalInput(msg)
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
			a.filterSelected = 0
			// If field has units, go to unit step; otherwise go to value step
			if field.Type == FilterTypeNumericWithUnit && len(field.Units) > 0 {
				a.filterStep = FilterStepUnit
			} else {
				a.filterStep = FilterStepValue
				a.filterInput = ""
				a.filterCursor = 0
			}
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

func (a *App) handleFilterUnitInput(msg tea.KeyMsg) tea.Cmd {
	field := a.getFieldByKey(a.pendingFilter.Field)
	if field == nil {
		return nil
	}

	switch msg.Type {
	case tea.KeyEscape:
		a.filterStep = FilterStepOperator
		a.filterSelected = 0
		return nil

	case tea.KeyEnter:
		if a.filterSelected < len(field.Units) {
			a.pendingFilter.Unit = field.Units[a.filterSelected]
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
		if a.filterSelected < len(field.Units)-1 {
			a.filterSelected++
		}
		return nil
	}
	return nil
}

func (a *App) handleFilterValueInput(msg tea.KeyMsg) tea.Cmd {
	field := a.getFieldByKey(a.pendingFilter.Field)

	switch msg.Type {
	case tea.KeyEscape:
		// Go back to previous step
		if field != nil && field.Type == FilterTypeNumericWithUnit {
			a.filterStep = FilterStepUnit
		} else {
			a.filterStep = FilterStepOperator
		}
		a.filterSelected = 0
		return nil

	case tea.KeyEnter:
		if a.filterInput != "" {
			a.pendingFilter.Value = a.filterInput
			a.filterStep = FilterStepLogical
			a.filterSelected = 0
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

func (a *App) handleFilterLogicalInput(msg tea.KeyMsg) tea.Cmd {
	// Options: Done (apply filter), && (add another with AND), || (add another with OR)
	options := []string{"Done", "&&", "||"}

	switch msg.Type {
	case tea.KeyEscape:
		a.filterStep = FilterStepValue
		return nil

	case tea.KeyEnter:
		switch a.filterSelected {
		case 0: // Done - apply filter and exit
			a.activeFilters = append(a.activeFilters, a.pendingFilter)
			a.pendingFilter = Filter{}
			a.filterInput = ""
			a.focus = a.prevFocus
			a.applyFilters()
		case 1: // && - add filter with AND and continue
			a.pendingFilter.LogicalOperator = "&&"
			a.activeFilters = append(a.activeFilters, a.pendingFilter)
			a.pendingFilter = Filter{}
			a.filterInput = ""
			a.filterStep = FilterStepField
			a.filterSelected = 0
			a.updateFilteredFields()
		case 2: // || - add filter with OR and continue
			a.pendingFilter.LogicalOperator = "||"
			a.activeFilters = append(a.activeFilters, a.pendingFilter)
			a.pendingFilter = Filter{}
			a.filterInput = ""
			a.filterStep = FilterStepField
			a.filterSelected = 0
			a.updateFilteredFields()
		}
		return nil

	case tea.KeyUp:
		if a.filterSelected > 0 {
			a.filterSelected--
		}
		return nil

	case tea.KeyDown:
		if a.filterSelected < len(options)-1 {
			a.filterSelected++
		}
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

// initReplayEdit initializes replay edit mode with request data
func (a *App) initReplayEdit(req ngrok.Request) {
	a.replayEditStep = ReplayEditStepMain
	a.replayEditSelected = 0
	a.replayEditMethod = req.Request.Method
	a.replayEditPath = req.Request.URI
	a.replayEditBody = req.Request.DecodeBody()
	a.replayEditCursor = 0
	a.replayEditInput = ""

	// Copy headers
	a.replayEditHeaders = nil
	for k, vals := range req.Request.Headers {
		// Skip some internal headers
		lowerK := strings.ToLower(k)
		if lowerK == "host" || lowerK == "content-length" ||
			strings.HasPrefix(lowerK, "x-forwarded") {
			continue
		}
		for _, v := range vals {
			a.replayEditHeaders = append(a.replayEditHeaders, HeaderEntry{Key: k, Value: v})
		}
	}
}

// handleReplayEditInput handles keyboard input in replay edit mode
func (a *App) handleReplayEditInput(msg tea.KeyMsg) tea.Cmd {
	switch a.replayEditStep {
	case ReplayEditStepMain:
		return a.handleReplayEditMain(msg)
	case ReplayEditStepMethod:
		return a.handleReplayEditMethod(msg)
	case ReplayEditStepPath:
		return a.handleReplayEditPath(msg)
	case ReplayEditStepHeaders:
		return a.handleReplayEditHeaders(msg)
	case ReplayEditStepHeaderEdit:
		return a.handleReplayEditHeaderEdit(msg)
	case ReplayEditStepBody:
		return a.handleReplayEditBody(msg)
	}
	return nil
}

func (a *App) handleReplayEditMain(msg tea.KeyMsg) tea.Cmd {
	// Main menu: Method, Path, Headers, Body, Send, Cancel
	menuItems := 6

	switch msg.Type {
	case tea.KeyEscape:
		a.focus = a.prevFocus
		return nil

	case tea.KeyEnter:
		switch a.replayEditSelected {
		case 0: // Method
			a.replayEditStep = ReplayEditStepMethod
			a.replayEditSelected = indexOf(httpMethods, a.replayEditMethod)
		case 1: // Path
			a.replayEditStep = ReplayEditStepPath
			a.replayEditInput = a.replayEditPath
			a.replayEditCursor = len(a.replayEditInput)
		case 2: // Headers
			a.replayEditStep = ReplayEditStepHeaders
			a.replayEditSelected = 0
		case 3: // Body
			a.replayEditStep = ReplayEditStepBody
			a.replayEditInput = a.replayEditBody
			a.replayEditCursor = len(a.replayEditInput)
		case 4: // Send
			return a.sendEditedRequest()
		case 5: // Cancel
			a.focus = a.prevFocus
		}
		return nil

	case tea.KeyUp:
		if a.replayEditSelected > 0 {
			a.replayEditSelected--
		}
		return nil

	case tea.KeyDown:
		if a.replayEditSelected < menuItems-1 {
			a.replayEditSelected++
		}
		return nil
	}
	return nil
}

func (a *App) handleReplayEditMethod(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEscape:
		a.replayEditStep = ReplayEditStepMain
		a.replayEditSelected = 0
		return nil

	case tea.KeyEnter:
		if a.replayEditSelected < len(httpMethods) {
			a.replayEditMethod = httpMethods[a.replayEditSelected]
		}
		a.replayEditStep = ReplayEditStepMain
		a.replayEditSelected = 0
		return nil

	case tea.KeyUp:
		if a.replayEditSelected > 0 {
			a.replayEditSelected--
		}
		return nil

	case tea.KeyDown:
		if a.replayEditSelected < len(httpMethods)-1 {
			a.replayEditSelected++
		}
		return nil
	}
	return nil
}

func (a *App) handleReplayEditPath(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEscape:
		a.replayEditStep = ReplayEditStepMain
		a.replayEditSelected = 1
		return nil

	case tea.KeyEnter:
		a.replayEditPath = a.replayEditInput
		a.replayEditStep = ReplayEditStepMain
		a.replayEditSelected = 1
		return nil

	case tea.KeyBackspace:
		if len(a.replayEditInput) > 0 && a.replayEditCursor > 0 {
			a.replayEditInput = a.replayEditInput[:a.replayEditCursor-1] + a.replayEditInput[a.replayEditCursor:]
			a.replayEditCursor--
		}
		return nil

	case tea.KeyLeft:
		if a.replayEditCursor > 0 {
			a.replayEditCursor--
		}
		return nil

	case tea.KeyRight:
		if a.replayEditCursor < len(a.replayEditInput) {
			a.replayEditCursor++
		}
		return nil

	case tea.KeyRunes:
		char := string(msg.Runes)
		a.replayEditInput = a.replayEditInput[:a.replayEditCursor] + char + a.replayEditInput[a.replayEditCursor:]
		a.replayEditCursor += len(char)
		return nil
	}
	return nil
}

func (a *App) handleReplayEditHeaders(msg tea.KeyMsg) tea.Cmd {
	// Headers list: each header + [Add New] + [Done]
	totalItems := len(a.replayEditHeaders) + 2

	switch msg.Type {
	case tea.KeyEscape:
		a.replayEditStep = ReplayEditStepMain
		a.replayEditSelected = 2
		return nil

	case tea.KeyEnter:
		if a.replayEditSelected < len(a.replayEditHeaders) {
			// Edit existing header
			a.replayHeaderIdx = a.replayEditSelected
			a.replayHeaderField = "key"
			a.replayEditInput = a.replayEditHeaders[a.replayHeaderIdx].Key
			a.replayEditCursor = len(a.replayEditInput)
			a.replayEditStep = ReplayEditStepHeaderEdit
		} else if a.replayEditSelected == len(a.replayEditHeaders) {
			// Add new header
			a.replayEditHeaders = append(a.replayEditHeaders, HeaderEntry{Key: "", Value: ""})
			a.replayHeaderIdx = len(a.replayEditHeaders) - 1
			a.replayHeaderField = "key"
			a.replayEditInput = ""
			a.replayEditCursor = 0
			a.replayEditStep = ReplayEditStepHeaderEdit
		} else {
			// Done
			a.replayEditStep = ReplayEditStepMain
			a.replayEditSelected = 2
		}
		return nil

	case tea.KeyBackspace, tea.KeyDelete:
		// Delete selected header
		if a.replayEditSelected < len(a.replayEditHeaders) {
			a.replayEditHeaders = append(a.replayEditHeaders[:a.replayEditSelected], a.replayEditHeaders[a.replayEditSelected+1:]...)
			if a.replayEditSelected >= len(a.replayEditHeaders) && a.replayEditSelected > 0 {
				a.replayEditSelected--
			}
		}
		return nil

	case tea.KeyUp:
		if a.replayEditSelected > 0 {
			a.replayEditSelected--
		}
		return nil

	case tea.KeyDown:
		if a.replayEditSelected < totalItems-1 {
			a.replayEditSelected++
		}
		return nil
	}
	return nil
}

func (a *App) handleReplayEditHeaderEdit(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEscape:
		a.replayEditStep = ReplayEditStepHeaders
		return nil

	case tea.KeyEnter:
		if a.replayHeaderField == "key" {
			a.replayEditHeaders[a.replayHeaderIdx].Key = a.replayEditInput
			a.replayHeaderField = "value"
			a.replayEditInput = a.replayEditHeaders[a.replayHeaderIdx].Value
			a.replayEditCursor = len(a.replayEditInput)
		} else {
			a.replayEditHeaders[a.replayHeaderIdx].Value = a.replayEditInput
			a.replayEditStep = ReplayEditStepHeaders
		}
		return nil

	case tea.KeyBackspace:
		if len(a.replayEditInput) > 0 && a.replayEditCursor > 0 {
			a.replayEditInput = a.replayEditInput[:a.replayEditCursor-1] + a.replayEditInput[a.replayEditCursor:]
			a.replayEditCursor--
		}
		return nil

	case tea.KeyLeft:
		if a.replayEditCursor > 0 {
			a.replayEditCursor--
		}
		return nil

	case tea.KeyRight:
		if a.replayEditCursor < len(a.replayEditInput) {
			a.replayEditCursor++
		}
		return nil

	case tea.KeyRunes:
		char := string(msg.Runes)
		a.replayEditInput = a.replayEditInput[:a.replayEditCursor] + char + a.replayEditInput[a.replayEditCursor:]
		a.replayEditCursor += len(char)
		return nil
	}
	return nil
}

func (a *App) handleReplayEditBody(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEscape:
		a.replayEditStep = ReplayEditStepMain
		a.replayEditSelected = 3
		return nil

	case tea.KeyEnter:
		// Enter adds newline
		a.replayEditInput = a.replayEditInput[:a.replayEditCursor] + "\n" + a.replayEditInput[a.replayEditCursor:]
		a.replayEditCursor++
		return nil

	case tea.KeyTab:
		// Tab to confirm body editing
		a.replayEditBody = a.replayEditInput
		a.replayEditStep = ReplayEditStepMain
		a.replayEditSelected = 3
		return nil

	case tea.KeyBackspace:
		if len(a.replayEditInput) > 0 && a.replayEditCursor > 0 {
			a.replayEditInput = a.replayEditInput[:a.replayEditCursor-1] + a.replayEditInput[a.replayEditCursor:]
			a.replayEditCursor--
		}
		return nil

	case tea.KeyLeft:
		if a.replayEditCursor > 0 {
			a.replayEditCursor--
		}
		return nil

	case tea.KeyRight:
		if a.replayEditCursor < len(a.replayEditInput) {
			a.replayEditCursor++
		}
		return nil

	case tea.KeyUp:
		// Move cursor up one line
		a.replayEditCursor = a.moveCursorVertical(a.replayEditInput, a.replayEditCursor, -1)
		return nil

	case tea.KeyDown:
		// Move cursor down one line
		a.replayEditCursor = a.moveCursorVertical(a.replayEditInput, a.replayEditCursor, 1)
		return nil

	case tea.KeyRunes:
		char := string(msg.Runes)
		a.replayEditInput = a.replayEditInput[:a.replayEditCursor] + char + a.replayEditInput[a.replayEditCursor:]
		a.replayEditCursor += len(char)
		return nil
	}
	return nil
}

// sendEditedRequest sends the edited request
func (a *App) sendEditedRequest() tea.Cmd {
	// Get base URL from tunnels
	baseURL := ""
	if len(a.tunnels) > 0 {
		baseURL = a.tunnels[0].PublicURL
	}
	if baseURL == "" {
		a.lastError = fmt.Errorf("no tunnel available")
		a.focus = a.prevFocus
		return nil
	}

	method := a.replayEditMethod
	url := baseURL + a.replayEditPath
	body := a.replayEditBody
	headers := make(map[string]string)
	for _, h := range a.replayEditHeaders {
		if h.Key != "" {
			headers[h.Key] = h.Value
		}
	}

	// Exit edit mode
	a.focus = a.prevFocus

	return func() tea.Msg {
		// Create HTTP request
		var reqBody io.Reader
		if body != "" {
			reqBody = strings.NewReader(body)
		}

		req, err := http.NewRequest(method, url, reqBody)
		if err != nil {
			return messages.ErrorMsg{Err: fmt.Errorf("failed to create request: %w", err)}
		}

		// Set headers
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		// Send request
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return messages.ErrorMsg{Err: fmt.Errorf("request failed: %w", err)}
		}
		defer resp.Body.Close()

		// Success - refresh requests to see the new one
		return messages.ReplayMsg{RequestID: "edited", Err: nil}
	}
}

// indexOf finds the index of a string in a slice
func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return 0
}

// moveCursorVertical moves cursor up or down in multiline text
func (a *App) moveCursorVertical(text string, cursor int, direction int) int {
	if text == "" {
		return 0
	}

	// Find current line and column
	lines := strings.Split(text, "\n")
	currentLine := 0
	currentCol := cursor
	charCount := 0

	for i, line := range lines {
		lineLen := len(line)
		if i < len(lines)-1 {
			lineLen++ // account for newline
		}
		if charCount+lineLen > cursor {
			currentLine = i
			currentCol = cursor - charCount
			break
		}
		charCount += lineLen
	}

	// Calculate target line
	targetLine := currentLine + direction
	if targetLine < 0 {
		targetLine = 0
	}
	if targetLine >= len(lines) {
		targetLine = len(lines) - 1
	}

	// Calculate new cursor position
	newCursor := 0
	for i := 0; i < targetLine; i++ {
		newCursor += len(lines[i]) + 1 // +1 for newline
	}

	// Try to maintain column position
	targetCol := currentCol
	if targetCol > len(lines[targetLine]) {
		targetCol = len(lines[targetLine])
	}
	newCursor += targetCol

	if newCursor > len(text) {
		newCursor = len(text)
	}

	return newCursor
}

// initHistoryView initializes the history view
func (a *App) initHistoryView() {
	if a.storage == nil {
		return
	}

	a.historySelectedSess = 0

	// Load sessions (exclude current session)
	sessions, err := a.storage.GetSessions()
	if err == nil {
		// Filter out current session
		a.historySessions = nil
		for _, s := range sessions {
			if s.ID != a.storage.CurrentSessionID() {
				a.historySessions = append(a.historySessions, s)
			}
		}
	}
}

// handleHistoryInput handles keyboard input in history view
func (a *App) handleHistoryInput(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEscape:
		a.focus = a.prevFocus
		return nil

	case tea.KeyEnter:
		if len(a.historySessions) > 0 {
			// Load selected session's requests into main view
			sess := a.historySessions[a.historySelectedSess]
			a.loadHistoricalSession(sess.ID)
			a.focus = FocusList
		}
		return nil

	case tea.KeyUp:
		if a.historySelectedSess > 0 {
			a.historySelectedSess--
		}
		return nil

	case tea.KeyDown:
		if a.historySelectedSess < len(a.historySessions)-1 {
			a.historySelectedSess++
		}
		return nil

	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "j":
			if a.historySelectedSess < len(a.historySessions)-1 {
				a.historySelectedSess++
			}
		case "k":
			if a.historySelectedSess > 0 {
				a.historySelectedSess--
			}
		}
		return nil
	}
	return nil
}

// loadHistoricalSession loads a historical session into the main view
func (a *App) loadHistoricalSession(sessionID string) {
	if a.storage == nil {
		return
	}

	histReqs, err := a.storage.GetSessionRequests(sessionID)
	if err != nil {
		return
	}

	// Convert storage.HistoryRequest to ngrok.Request for display
	a.requests = nil
	for _, hr := range histReqs {
		req := ngrok.Request{
			ID:       hr.ID,
			Start:    hr.Timestamp,
			Duration: hr.DurationMS * 1_000_000, // ms to ns
			Request: ngrok.HTTPData{
				Method:  hr.Method,
				URI:     hr.Path,
				Headers: hr.ReqHeaders,
			},
			Response: ngrok.HTTPData{
				StatusCode: hr.StatusCode,
				Headers:    hr.ResHeaders,
			},
		}
		// Store body data for later retrieval
		req.Request.Raw = hr.ReqBody
		req.Response.Raw = hr.ResBody
		a.requests = append(a.requests, req)
	}

	a.viewingHistory = true
	a.viewingSessionID = sessionID
	a.selected = 0
	a.applyFilters()
	a.updateDetailViewport()
}

// exitHistoryView returns to live view
func (a *App) exitHistoryView() {
	a.viewingHistory = false
	a.viewingSessionID = ""
	// Requests will be refreshed on next poll
}

// initDiffView initializes the diff view
func (a *App) initDiffView() {
	a.diffViewport = viewport.New(0, 0)
	a.diffViewport.Style = lipgloss.NewStyle()
}

// handleDiffInput handles keyboard input in diff view
func (a *App) handleDiffInput(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEscape:
		a.diffRequestA = nil
		a.diffRequestB = nil
		a.focus = a.prevFocus
		return nil

	case tea.KeyUp, tea.KeyRunes:
		if msg.Type == tea.KeyRunes && string(msg.Runes) == "k" {
			a.diffViewport.LineUp(1)
		} else if msg.Type == tea.KeyUp {
			a.diffViewport.LineUp(1)
		}
		return nil

	case tea.KeyDown:
		a.diffViewport.LineDown(1)
		return nil
	}

	if msg.Type == tea.KeyRunes {
		switch string(msg.Runes) {
		case "j":
			a.diffViewport.LineDown(1)
		case "g":
			a.diffViewport.GotoTop()
		case "G":
			a.diffViewport.GotoBottom()
		}
	}
	return nil
}

// generateDiff generates a diff between two requests
func (a *App) generateDiff() string {
	if a.diffRequestA == nil || a.diffRequestB == nil {
		return "No requests selected for diff"
	}

	reqA := a.diffRequestA
	reqB := a.diffRequestB

	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	addedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))   // green
	removedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")) // red
	unchangedStyle := lipgloss.NewStyle().Foreground(ColorMuted)
	labelStyle := lipgloss.NewStyle().Bold(true)

	sb.WriteString(titleStyle.Render("Request Diff"))
	sb.WriteString("\n")
	timeA := reqA.Start.Format("15:04:05")
	timeB := reqB.Start.Format("15:04:05")
	sb.WriteString(fmt.Sprintf("A: %s %s (%s)\n", reqA.Request.Method, reqA.Request.URI, timeA))
	sb.WriteString(fmt.Sprintf("B: %s %s (%s)\n", reqB.Request.Method, reqB.Request.URI, timeB))
	sb.WriteString("\n")

	// Method diff
	sb.WriteString(labelStyle.Render("Method: "))
	if reqA.Request.Method != reqB.Request.Method {
		sb.WriteString(removedStyle.Render("- "+reqA.Request.Method) + " ")
		sb.WriteString(addedStyle.Render("+ " + reqB.Request.Method))
	} else {
		sb.WriteString(unchangedStyle.Render(reqA.Request.Method))
	}
	sb.WriteString("\n")

	// Path diff
	sb.WriteString(labelStyle.Render("Path: "))
	if reqA.Request.URI != reqB.Request.URI {
		sb.WriteString("\n")
		sb.WriteString(removedStyle.Render("  - "+reqA.Request.URI) + "\n")
		sb.WriteString(addedStyle.Render("  + " + reqB.Request.URI))
	} else {
		sb.WriteString(unchangedStyle.Render(reqA.Request.URI))
	}
	sb.WriteString("\n")

	// Status diff
	sb.WriteString(labelStyle.Render("Status: "))
	statusCodeA := reqA.StatusCode()
	statusCodeB := reqB.StatusCode()
	statusA := fmt.Sprintf("%d %s", statusCodeA, httpStatusText(statusCodeA))
	statusB := fmt.Sprintf("%d %s", statusCodeB, httpStatusText(statusCodeB))
	if statusA != statusB {
		sb.WriteString(removedStyle.Render("- "+statusA) + " ")
		sb.WriteString(addedStyle.Render("+ " + statusB))
	} else {
		sb.WriteString(unchangedStyle.Render(statusA))
	}
	sb.WriteString("\n")

	// Duration diff
	sb.WriteString(labelStyle.Render("Duration: "))
	durA := fmt.Sprintf("%dms", reqA.Duration/1_000_000)
	durB := fmt.Sprintf("%dms", reqB.Duration/1_000_000)
	if durA != durB {
		sb.WriteString(removedStyle.Render("- "+durA) + " ")
		sb.WriteString(addedStyle.Render("+ " + durB))
	} else {
		sb.WriteString(unchangedStyle.Render(durA))
	}
	sb.WriteString("\n\n")

	// Request Headers diff
	sb.WriteString(labelStyle.Render("Request Headers:"))
	sb.WriteString("\n")
	sb.WriteString(a.diffHeaders(reqA.Request.Headers, reqB.Request.Headers, addedStyle, removedStyle, unchangedStyle))
	sb.WriteString("\n")

	// Request Body diff
	bodyA := reqA.Request.DecodeBody()
	bodyB := reqB.Request.DecodeBody()
	if bodyA != "" || bodyB != "" {
		sb.WriteString(labelStyle.Render("Request Body:"))
		sb.WriteString("\n")
		sb.WriteString(a.diffText(bodyA, bodyB, addedStyle, removedStyle, unchangedStyle))
		sb.WriteString("\n")
	}

	// Response Headers diff
	sb.WriteString(labelStyle.Render("Response Headers:"))
	sb.WriteString("\n")
	sb.WriteString(a.diffHeaders(reqA.Response.Headers, reqB.Response.Headers, addedStyle, removedStyle, unchangedStyle))
	sb.WriteString("\n")

	// Response Body diff
	respBodyA := reqA.Response.DecodeBody()
	respBodyB := reqB.Response.DecodeBody()
	if respBodyA != "" || respBodyB != "" {
		sb.WriteString(labelStyle.Render("Response Body:"))
		sb.WriteString("\n")
		sb.WriteString(a.diffText(respBodyA, respBodyB, addedStyle, removedStyle, unchangedStyle))
	}

	return sb.String()
}

// diffHeaders generates a diff for headers
func (a *App) diffHeaders(headersA, headersB map[string][]string, addedStyle, removedStyle, unchangedStyle lipgloss.Style) string {
	var sb strings.Builder

	// Collect all keys
	allKeys := make(map[string]bool)
	for k := range headersA {
		allKeys[k] = true
	}
	for k := range headersB {
		allKeys[k] = true
	}

	// Sort keys
	var keys []string
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		valsA := headersA[k]
		valsB := headersB[k]

		valA := strings.Join(valsA, ", ")
		valB := strings.Join(valsB, ", ")

		if len(valsA) == 0 {
			// Added in B
			sb.WriteString(addedStyle.Render(fmt.Sprintf("  + %s: %s", k, valB)))
			sb.WriteString("\n")
		} else if len(valsB) == 0 {
			// Removed in B
			sb.WriteString(removedStyle.Render(fmt.Sprintf("  - %s: %s", k, valA)))
			sb.WriteString("\n")
		} else if valA != valB {
			// Changed
			sb.WriteString(removedStyle.Render(fmt.Sprintf("  - %s: %s", k, valA)))
			sb.WriteString("\n")
			sb.WriteString(addedStyle.Render(fmt.Sprintf("  + %s: %s", k, valB)))
			sb.WriteString("\n")
		} else {
			// Unchanged
			sb.WriteString(unchangedStyle.Render(fmt.Sprintf("    %s: %s", k, valA)))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// diffText generates a simple line-by-line diff for text content
func (a *App) diffText(textA, textB string, addedStyle, removedStyle, unchangedStyle lipgloss.Style) string {
	if textA == textB {
		// Show truncated if same
		if len(textA) > 200 {
			return unchangedStyle.Render("  (identical, " + fmt.Sprintf("%d bytes", len(textA)) + ")\n")
		}
		lines := strings.Split(textA, "\n")
		var sb strings.Builder
		for _, line := range lines {
			sb.WriteString(unchangedStyle.Render("    " + line))
			sb.WriteString("\n")
		}
		return sb.String()
	}

	linesA := strings.Split(textA, "\n")
	linesB := strings.Split(textB, "\n")

	var sb strings.Builder

	// Simple line-by-line comparison (not a full diff algorithm)
	maxLines := len(linesA)
	if len(linesB) > maxLines {
		maxLines = len(linesB)
	}

	// Limit output for very long diffs
	if maxLines > 50 {
		sb.WriteString(fmt.Sprintf("  (showing first 50 of %d lines)\n", maxLines))
		maxLines = 50
	}

	for i := 0; i < maxLines; i++ {
		lineA := ""
		lineB := ""
		if i < len(linesA) {
			lineA = linesA[i]
		}
		if i < len(linesB) {
			lineB = linesB[i]
		}

		if lineA == lineB {
			sb.WriteString(unchangedStyle.Render("    " + lineA))
			sb.WriteString("\n")
		} else {
			if lineA != "" {
				sb.WriteString(removedStyle.Render("  - " + lineA))
				sb.WriteString("\n")
			}
			if lineB != "" {
				sb.WriteString(addedStyle.Render("  + " + lineB))
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
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

// matchesAllFilters checks if a request matches all active filters with AND/OR logic
func (a *App) matchesAllFilters(req ngrok.Request) bool {
	if len(a.activeFilters) == 0 {
		return true
	}

	// Process filters with AND/OR logic
	result := a.matchesFilter(req, a.activeFilters[0])

	for i := 1; i < len(a.activeFilters); i++ {
		prevFilter := a.activeFilters[i-1]
		currentMatch := a.matchesFilter(req, a.activeFilters[i])

		if prevFilter.LogicalOperator == "||" {
			result = result || currentMatch
		} else {
			// Default to AND
			result = result && currentMatch
		}
	}

	return result
}

// matchesFilter checks if a request matches a single filter
func (a *App) matchesFilter(req ngrok.Request, f Filter) bool {
	switch f.Field {
	case "status":
		return a.compareStringOp(fmt.Sprintf("%d", req.StatusCode()), f.Operator, f.Value)
	case "path":
		return a.compareStringOp(req.Request.URI, f.Operator, f.Value)
	case "duration":
		return a.compareDuration(req.DurationMs(), f.Operator, f.Unit, f.Value)
	case "response_size":
		return a.compareSize(req.ResponseSize(), f.Operator, f.Unit, f.Value)
	default:
		// Handle headers
		if strings.HasPrefix(f.Field, "header.") {
			headerName := strings.TrimPrefix(f.Field, "header.")
			headerValue := a.getHeaderValue(req, headerName)
			return a.compareStringOp(headerValue, f.Operator, f.Value)
		}
	}
	return true
}

// getHeaderValue gets a header value from request (case-insensitive)
func (a *App) getHeaderValue(req ngrok.Request, headerName string) string {
	headerName = strings.ToLower(headerName)
	for k, vals := range req.Request.Headers {
		if strings.ToLower(k) == headerName && len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}

// compareStringOp compares strings with operators ==, !=, match, !match
func (a *App) compareStringOp(val string, op string, target string) bool {
	switch op {
	case "==":
		return val == target
	case "!=":
		return val != target
	case "match":
		return strings.Contains(strings.ToLower(val), strings.ToLower(target))
	case "!match":
		return !strings.Contains(strings.ToLower(val), strings.ToLower(target))
	}
	return false
}

// compareDuration compares duration with unit conversion
func (a *App) compareDuration(valMs float64, op string, unit string, target string) bool {
	t, err := strconv.ParseFloat(target, 64)
	if err != nil {
		return false
	}

	// Convert target to milliseconds based on unit
	var targetMs float64
	switch unit {
	case "ms":
		targetMs = t
	case "s":
		targetMs = t * 1000
	case "m":
		targetMs = t * 60 * 1000
	case "h":
		targetMs = t * 60 * 60 * 1000
	case "d":
		targetMs = t * 24 * 60 * 60 * 1000
	default:
		targetMs = t
	}

	return a.compareFloat(valMs, op, targetMs)
}

// compareSize compares size with unit conversion
func (a *App) compareSize(valBytes int, op string, unit string, target string) bool {
	t, err := strconv.ParseFloat(target, 64)
	if err != nil {
		return false
	}

	// Convert target to bytes based on unit
	var targetBytes float64
	switch unit {
	case "b":
		targetBytes = t
	case "kb":
		targetBytes = t * 1024
	case "mb":
		targetBytes = t * 1024 * 1024
	default:
		targetBytes = t
	}

	return a.compareFloat(float64(valBytes), op, targetBytes)
}

// compareFloat compares two float values
func (a *App) compareFloat(val float64, op string, target float64) bool {
	switch op {
	case ">":
		return val > target
	case "<":
		return val < target
	case ">=":
		return val >= target
	case "<=":
		return val <= target
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

	if a.viewingHistory {
		// Show history mode indicator
		tunnelInfo = lipgloss.NewStyle().
			Background(lipgloss.Color("#7C3AED")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1).
			Render(" 📜 Viewing History - press 'h' to return to live ")
	} else if len(a.tunnels) > 0 {
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
	var info string
	if a.viewingHistory {
		info = tunnelInfo
	} else {
		info = lipgloss.NewStyle().
			Background(lipgloss.Color("#1F2937")).
			Padding(0, 1).
			Render(tunnelInfo)
	}

	headerContent := lipgloss.JoinHorizontal(lipgloss.Center, title, info)

	return lipgloss.NewStyle().
		Width(a.width).
		Render(headerContent)
}

// renderContent renders the main content area
func (a *App) renderContent() string {
	contentHeight := a.height - 4 // header + footer

	// History view takes full screen
	if a.focus == FocusHistory {
		return a.renderHistoryView(a.width, contentHeight)
	}

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
	if a.focus == FocusList || a.focus == FocusFilter || a.focus == FocusReplayEdit {
		listBorder = ActiveBorderStyle
	} else if a.focus == FocusDetailPanel || a.focus == FocusDiff {
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
	if a.focus == FocusList || a.focus == FocusFilter || a.focus == FocusReplayEdit {
		listBorder = ActiveBorderStyle
	} else if a.focus == FocusDetailPanel || a.focus == FocusDiff {
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

	// If in replay edit mode, show edit UI
	if a.focus == FocusReplayEdit {
		return a.renderReplayEditInPanel(width, height)
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

	// Show current filter chain being built
	if len(a.activeFilters) > 0 {
		var filterChain string
		for i, f := range a.activeFilters {
			if i > 0 {
				filterChain += " "
			}
			filterChain += a.formatFilterBadge(f)
			if f.LogicalOperator != "" {
				filterChain += " " + f.LogicalOperator
			}
		}
		lines = append(lines, mutedStyle.Render("Current: ")+filterChain)
		lines = append(lines, "")
	}

	switch a.filterStep {
	case FilterStepField:
		lines = append(lines, titleStyle.Render("Select Field"))
		lines = append(lines, "")

		if a.filterInput != "" {
			lines = append(lines, mutedStyle.Render("Search: ")+a.filterInput+"█")
			lines = append(lines, "")
		}

		// Show limited number of fields to fit in panel
		maxVisible := height - 6
		if maxVisible < 5 {
			maxVisible = 5
		}
		startIdx := 0
		if a.filterSelected >= maxVisible {
			startIdx = a.filterSelected - maxVisible + 1
		}
		endIdx := min(startIdx+maxVisible, len(a.filteredFields))

		for i := startIdx; i < endIdx; i++ {
			f := a.filteredFields[i]
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

	case FilterStepUnit:
		field := a.getFieldByKey(a.pendingFilter.Field)
		if field != nil {
			lines = append(lines, titleStyle.Render("Select Unit"))
			lines = append(lines, mutedStyle.Render(fmt.Sprintf("%s %s", field.Name, a.pendingFilter.Operator)))
			lines = append(lines, "")

			for i, unit := range field.Units {
				if i == a.filterSelected {
					lines = append(lines, selectedStyle.Render("▶ "+unit))
				} else {
					lines = append(lines, "  "+unit)
				}
			}
		}

	case FilterStepValue:
		field := a.getFieldByKey(a.pendingFilter.Field)
		if field != nil {
			filterDesc := field.Name + " " + a.pendingFilter.Operator
			if a.pendingFilter.Unit != "" {
				filterDesc += " (" + a.pendingFilter.Unit + ")"
			}
			lines = append(lines, titleStyle.Render("Enter Value"))
			lines = append(lines, mutedStyle.Render(filterDesc))
			lines = append(lines, "")
			lines = append(lines, "> "+a.filterInput+"█")
		}

	case FilterStepLogical:
		field := a.getFieldByKey(a.pendingFilter.Field)
		if field != nil {
			filterDesc := a.formatFilterBadge(a.pendingFilter)
			lines = append(lines, titleStyle.Render("Add Another Filter?"))
			lines = append(lines, mutedStyle.Render("Filter: "+filterDesc))
			lines = append(lines, "")

			options := []string{"Done (apply filter)", "&& (AND another)", "|| (OR another)"}
			for i, opt := range options {
				if i == a.filterSelected {
					lines = append(lines, selectedStyle.Render("▶ "+opt))
				} else {
					lines = append(lines, "  "+opt)
				}
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("↑↓: select  Enter: confirm  Esc: back"))

	return strings.Join(lines, "\n")
}

// formatFilterBadge formats a filter as a display string
func (a *App) formatFilterBadge(f Filter) string {
	result := f.Field + " " + f.Operator
	if f.Unit != "" {
		result += " " + f.Value + f.Unit
	} else {
		result += " " + f.Value
	}
	return result
}

// renderReplayEditInPanel renders the replay edit UI inside the request list panel
func (a *App) renderReplayEditInPanel(width, height int) string {
	var lines []string

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	mutedStyle := lipgloss.NewStyle().Foreground(ColorMuted)
	selectedStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))

	switch a.replayEditStep {
	case ReplayEditStepMain:
		lines = append(lines, titleStyle.Render("Replay with Edit"))
		lines = append(lines, "")

		menuItems := []struct {
			label string
			value string
		}{
			{"Method", a.replayEditMethod},
			{"Path", a.replayEditPath},
			{"Headers", fmt.Sprintf("(%d)", len(a.replayEditHeaders))},
			{"Body", fmt.Sprintf("(%d bytes)", len(a.replayEditBody))},
			{"► Send Request", ""},
			{"✕ Cancel", ""},
		}

		for i, item := range menuItems {
			line := ""
			if i == a.replayEditSelected {
				line = selectedStyle.Render("▶ " + item.label)
			} else {
				line = "  " + item.label
			}
			if item.value != "" {
				line += "  " + valueStyle.Render(item.value)
			}
			lines = append(lines, line)
		}

	case ReplayEditStepMethod:
		lines = append(lines, titleStyle.Render("Select Method"))
		lines = append(lines, "")

		for i, method := range httpMethods {
			if i == a.replayEditSelected {
				lines = append(lines, selectedStyle.Render("▶ "+method))
			} else {
				lines = append(lines, "  "+method)
			}
		}

	case ReplayEditStepPath:
		lines = append(lines, titleStyle.Render("Edit Path"))
		lines = append(lines, "")
		// Show input with cursor
		input := a.replayEditInput
		if a.replayEditCursor < len(input) {
			input = input[:a.replayEditCursor] + "█" + input[a.replayEditCursor:]
		} else {
			input = input + "█"
		}
		lines = append(lines, "> "+input)

	case ReplayEditStepHeaders:
		lines = append(lines, titleStyle.Render("Edit Headers"))
		lines = append(lines, mutedStyle.Render("Enter: edit  Backspace: delete"))
		lines = append(lines, "")

		maxVisible := height - 6
		if maxVisible < 3 {
			maxVisible = 3
		}

		totalItems := len(a.replayEditHeaders) + 2
		startIdx := 0
		if a.replayEditSelected >= maxVisible {
			startIdx = a.replayEditSelected - maxVisible + 1
		}
		endIdx := min(startIdx+maxVisible, totalItems)

		for i := startIdx; i < endIdx; i++ {
			var line string
			if i < len(a.replayEditHeaders) {
				h := a.replayEditHeaders[i]
				headerStr := h.Key + ": " + h.Value
				if len(headerStr) > width-4 {
					headerStr = headerStr[:width-7] + "..."
				}
				if i == a.replayEditSelected {
					line = selectedStyle.Render("▶ " + headerStr)
				} else {
					line = "  " + headerStr
				}
			} else if i == len(a.replayEditHeaders) {
				if i == a.replayEditSelected {
					line = selectedStyle.Render("▶ [Add New Header]")
				} else {
					line = "  [Add New Header]"
				}
			} else {
				if i == a.replayEditSelected {
					line = selectedStyle.Render("▶ [Done]")
				} else {
					line = "  [Done]"
				}
			}
			lines = append(lines, line)
		}

	case ReplayEditStepHeaderEdit:
		fieldName := "Key"
		if a.replayHeaderField == "value" {
			fieldName = "Value"
		}
		lines = append(lines, titleStyle.Render("Edit Header "+fieldName))
		lines = append(lines, "")
		input := a.replayEditInput
		if a.replayEditCursor < len(input) {
			input = input[:a.replayEditCursor] + "█" + input[a.replayEditCursor:]
		} else {
			input = input + "█"
		}
		lines = append(lines, "> "+input)

	case ReplayEditStepBody:
		lines = append(lines, titleStyle.Render("Edit Body"))
		lines = append(lines, "")

		// Show body with cursor at position
		input := a.replayEditInput
		cursorPos := a.replayEditCursor
		if cursorPos > len(input) {
			cursorPos = len(input)
		}

		// Insert cursor character at position
		var displayText string
		if cursorPos < len(input) {
			displayText = input[:cursorPos] + "█" + input[cursorPos:]
		} else {
			displayText = input + "█"
		}

		// Split into lines and display
		bodyLines := strings.Split(displayText, "\n")
		maxBodyLines := height - 5
		if maxBodyLines < 3 {
			maxBodyLines = 3
		}

		for i, bl := range bodyLines {
			if i >= maxBodyLines {
				lines = append(lines, mutedStyle.Render(fmt.Sprintf("... (%d more lines)", len(bodyLines)-maxBodyLines)))
				break
			}
			if len(bl) > width-2 {
				bl = bl[:width-5] + "..."
			}
			lines = append(lines, bl)
		}

		lines = append(lines, "")
		lines = append(lines, mutedStyle.Render("Tab: save  Esc: cancel"))
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("↑↓: select  Enter: confirm  Esc: back"))

	return strings.Join(lines, "\n")
}

// renderRequestLine renders a single request line (compact mode)
func (a *App) renderRequestLine(req ngrok.Request, width int, selected bool) string {
	statusCode := req.StatusCode()
	timeAgo := formatRelativeTime(req.Start)

	// Check if this is a diff-selected request
	isDiffA := a.diffRequestA != nil && a.diffRequestA.ID == req.ID
	isDiffB := a.diffRequestB != nil && a.diffRequestB.ID == req.ID

	// Compact format: "▶ METHOD  STATUS PATH         TIME"
	// Widths:          2  8       4     var          6
	// METHOD is 8 chars to fit "OPTIONS" (7) + space
	// Extra 4 chars for [A]/[B] marker when diff is active
	extraWidth := 0
	if a.diffRequestA != nil || a.diffRequestB != nil {
		extraWidth = 4
	}
	fixedWidth := 2 + 8 + 4 + 6 + extraWidth
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

	// Add diff marker
	var diffMarker string
	if a.diffRequestA != nil || a.diffRequestB != nil {
		if isDiffA {
			diffMarker = lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")).Bold(true).Render("[A] ")
		} else if isDiffB {
			diffMarker = lipgloss.NewStyle().Foreground(lipgloss.Color("#60A5FA")).Bold(true).Render("[B] ")
		} else {
			diffMarker = "    "
		}
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

	return fmt.Sprintf("%s%s%s%s%s%s", indicator, diffMarker, method, status, path, time)
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
	// If in diff mode, show diff view
	if a.focus == FocusDiff {
		return a.renderDiffView(width, height)
	}

	if len(a.filteredReqs) == 0 || a.selected >= len(a.filteredReqs) {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
			"Select a request to view details")
	}

	// Use viewport for scrollable content
	return a.detailViewport.View()
}

// renderDiffView renders the diff comparison view
func (a *App) renderDiffView(width, height int) string {
	if a.diffRequestA == nil || a.diffRequestB == nil {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
			"No diff to display")
	}

	// Update viewport size if needed
	if a.diffViewport.Width != width || a.diffViewport.Height != height {
		a.diffViewport.Width = width
		a.diffViewport.Height = height
	}

	// Generate diff content
	content := a.generateDiff()

	// Wrap content to fit width
	wrappedContent := lipgloss.NewStyle().Width(width).Render(content)
	a.diffViewport.SetContent(wrappedContent)

	return a.diffViewport.View()
}

// renderHistoryView renders the history browser view
func (a *App) renderHistoryView(width, height int) string {
	if a.storage == nil {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
			"Storage not available")
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	selectedStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(ColorMuted)

	var lines []string

	lines = append(lines, titleStyle.Render("History - Select Session"))
	lines = append(lines, "")

	if len(a.historySessions) == 0 {
		lines = append(lines, mutedStyle.Render("No previous sessions found"))
	} else {
		maxVisible := height - 6
		startIdx := 0
		if a.historySelectedSess >= maxVisible {
			startIdx = a.historySelectedSess - maxVisible + 1
		}
		endIdx := min(startIdx+maxVisible, len(a.historySessions))

		for i := startIdx; i < endIdx; i++ {
			sess := a.historySessions[i]
			dateStr := sess.StartedAt.Format("Jan 02, 15:04")

			// Count requests in session
			reqs, _ := a.storage.GetSessionRequests(sess.ID)
			reqCount := len(reqs)

			line := fmt.Sprintf("%s (%d requests)", dateStr, reqCount)
			if sess.TunnelURL != "" {
				// Truncate URL if too long
				url := sess.TunnelURL
				maxURLLen := width - len(line) - 10
				if maxURLLen > 20 && len(url) > maxURLLen {
					url = url[:maxURLLen-3] + "..."
				}
				line += " - " + url
			}

			if i == a.historySelectedSess {
				lines = append(lines, selectedStyle.Render("▶ "+line))
			} else {
				lines = append(lines, "  "+line)
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("j/k: nav  Enter: load session  Esc: back"))

	content := strings.Join(lines, "\n")

	return BorderStyle.Width(width - 2).Height(height - 2).Render(content)
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
	for i, f := range a.activeFilters {
		// Format filter value with unit if present
		value := f.Value
		if f.Unit != "" {
			value += f.Unit
		}
		badge := lipgloss.NewStyle().
			Background(ColorPrimary).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1).
			Render(fmt.Sprintf("%s %s %s", f.Field, f.Operator, value))
		statusParts = append(statusParts, badge)

		// Show logical operator if not the last filter
		if f.LogicalOperator != "" && i < len(a.activeFilters)-1 {
			opStyle := lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
			statusParts = append(statusParts, opStyle.Render(f.LogicalOperator))
		}
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

	// Show diff mode indicator
	if a.diffRequestA != nil && a.focus != FocusDiff {
		diffBadge := lipgloss.NewStyle().
			Background(lipgloss.Color("#FBBF24")).
			Foreground(lipgloss.Color("#000000")).
			Padding(0, 1).
			Render("Diff: [A] selected, press 'd' on another request")
		statusParts = append(statusParts, diffBadge)
	}

	// Help text
	var help string
	if a.focus == FocusFilter {
		help = fmt.Sprintf("%s select  %s confirm  %s cancel",
			HelpKeyStyle.Render("↑↓"),
			HelpKeyStyle.Render("enter"),
			HelpKeyStyle.Render("esc"))
	} else if a.focus == FocusReplayEdit {
		if a.replayEditStep == ReplayEditStepBody {
			help = fmt.Sprintf("%s save  %s cancel",
				HelpKeyStyle.Render("tab"),
				HelpKeyStyle.Render("esc"))
		} else if a.replayEditStep == ReplayEditStepPath || a.replayEditStep == ReplayEditStepHeaderEdit {
			help = fmt.Sprintf("%s move  %s confirm  %s cancel",
				HelpKeyStyle.Render("←→"),
				HelpKeyStyle.Render("enter"),
				HelpKeyStyle.Render("esc"))
		} else {
			help = fmt.Sprintf("%s select  %s confirm  %s back/cancel",
				HelpKeyStyle.Render("↑↓"),
				HelpKeyStyle.Render("enter"),
				HelpKeyStyle.Render("esc"))
		}
	} else if a.focus == FocusDiff {
		help = fmt.Sprintf("%s scroll  %s close",
			HelpKeyStyle.Render("j/k/mouse"),
			HelpKeyStyle.Render("esc"))
	} else if a.focus == FocusHistory {
		help = fmt.Sprintf("%s nav  %s load session  %s back",
			HelpKeyStyle.Render("j/k"),
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
		if a.diffRequestA != nil {
			// Diff mode: show instruction to select second request
			help = fmt.Sprintf("%s nav  %s select B for diff  %s cancel diff  %s quit",
				HelpKeyStyle.Render("j/k"),
				HelpKeyStyle.Render("d"),
				HelpKeyStyle.Render("esc"),
				HelpKeyStyle.Render("q"))
		} else if a.viewingHistory {
			help = fmt.Sprintf("%s nav  %s search  %s filter  %s live  %s copy  %s diff  %s quit",
				HelpKeyStyle.Render("j/k"),
				HelpKeyStyle.Render("/"),
				HelpKeyStyle.Render("f"),
				HelpKeyStyle.Render("h"),
				HelpKeyStyle.Render("c"),
				HelpKeyStyle.Render("d"),
				HelpKeyStyle.Render("q"))
		} else {
			help = fmt.Sprintf("%s nav  %s search  %s filter  %s replay  %s replay with edit  %s copy  %s diff  %s history  %s quit",
				HelpKeyStyle.Render("j/k"),
				HelpKeyStyle.Render("/"),
				HelpKeyStyle.Render("f"),
				HelpKeyStyle.Render("r"),
				HelpKeyStyle.Render("R"),
				HelpKeyStyle.Render("c"),
				HelpKeyStyle.Render("d"),
				HelpKeyStyle.Render("h"),
				HelpKeyStyle.Render("q"))
		}
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
		detailWidth = a.width - 4 // border + padding
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

// saveNewRequests saves any new requests to persistent storage
func (a *App) saveNewRequests() {
	if a.storage == nil || a.storage.CurrentSessionID() == "" {
		return
	}

	for _, req := range a.requests {
		// Skip if already saved
		if a.savedReqIDs[req.ID] {
			continue
		}

		// Convert to storage format and save
		histReq := storage.HistoryRequest{
			ID:         req.ID,
			SessionID:  a.storage.CurrentSessionID(),
			Method:     req.Request.Method,
			Path:       req.Request.URI,
			StatusCode: req.StatusCode(),
			DurationMS: req.Duration / 1_000_000, // nanoseconds to milliseconds
			Timestamp:  req.Start,
			ReqHeaders: req.Request.Headers,
			ReqBody:    req.Request.DecodeBody(),
			ResHeaders: req.Response.Headers,
			ResBody:    req.Response.DecodeBody(),
		}

		if err := a.storage.SaveRequest(histReq); err == nil {
			a.savedReqIDs[req.ID] = true
		}
	}
}

// CloseStorage closes the storage connection
func (a *App) CloseStorage() {
	if a.storage != nil {
		a.storage.Close()
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
