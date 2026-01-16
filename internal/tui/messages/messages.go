package messages

import (
	"time"

	"github.com/sung01299/mole/internal/ngrok"
)

// TickMsg is sent periodically to trigger data refresh
type TickMsg struct {
	Time time.Time
}

// TunnelsMsg contains fetched tunnel data
type TunnelsMsg struct {
	Tunnels []ngrok.Tunnel
	Err     error
}

// RequestsMsg contains fetched request data
type RequestsMsg struct {
	Requests []ngrok.Request
	Err      error
}

// ReplayMsg indicates the result of a replay action
type ReplayMsg struct {
	RequestID string
	Err       error
}

// WindowFocusMsg indicates whether the window has focus
type WindowFocusMsg struct {
	Focused bool
}

// ErrorMsg represents an error to be displayed
type ErrorMsg struct {
	Err error
}

// CopyMsg indicates the result of a copy to clipboard action
type CopyMsg struct {
	Success bool
}
