package messages

import (
	"time"

	"github.com/mole-cli/mole/internal/ngrok"
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
