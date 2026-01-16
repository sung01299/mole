package ngrok

import "time"

// Tunnel represents an ngrok tunnel
type Tunnel struct {
	Name      string `json:"name"`
	URI       string `json:"uri"`
	PublicURL string `json:"public_url"`
	Proto     string `json:"proto"`
	Config    struct {
		Addr    string `json:"addr"`
		Inspect bool   `json:"inspect"`
	} `json:"config"`
	Metrics struct {
		Conns struct {
			Count  int     `json:"count"`
			Gauge  int     `json:"gauge"`
			Rate1  float64 `json:"rate1"`
			Rate5  float64 `json:"rate5"`
			Rate15 float64 `json:"rate15"`
			P50    float64 `json:"p50"`
			P90    float64 `json:"p90"`
			P99    float64 `json:"p99"`
		} `json:"conns"`
		HTTP struct {
			Count  int     `json:"count"`
			Rate1  float64 `json:"rate1"`
			Rate5  float64 `json:"rate5"`
			Rate15 float64 `json:"rate15"`
			P50    float64 `json:"p50"`
			P90    float64 `json:"p90"`
			P99    float64 `json:"p99"`
		} `json:"http"`
	} `json:"metrics"`
}

// TunnelsResponse is the response from GET /api/tunnels
type TunnelsResponse struct {
	Tunnels []Tunnel `json:"tunnels"`
	URI     string   `json:"uri"`
}

// Request represents a captured HTTP request
type Request struct {
	URI            string    `json:"uri"`
	ID             string    `json:"id"`
	TunnelName     string    `json:"tunnel_name"`
	RemoteAddr     string    `json:"remote_addr"`
	Start          time.Time `json:"start"`
	Duration       int64     `json:"duration"` // nanoseconds
	Request        HTTPData  `json:"request"`
	Response       HTTPData  `json:"response"`
	ResponseStatus string    `json:"response_status"` // e.g., "200 OK"
}

// HTTPData represents HTTP request or response data
type HTTPData struct {
	Method  string              `json:"method,omitempty"`
	Proto   string              `json:"proto"`
	Headers map[string][]string `json:"headers"`
	URI     string              `json:"uri,omitempty"`
	Raw     string              `json:"raw"`
}

// RequestsResponse is the response from GET /api/requests/http
type RequestsResponse struct {
	Requests []Request `json:"requests"`
	URI      string    `json:"uri"`
}

// ReplayRequest is the request body for POST /api/requests/http/{id}/replay
type ReplayRequest struct {
	ID     string `json:"id"`
	Target string `json:"target,omitempty"` // optional: override target address
}

// StatusCode extracts the numeric status code from ResponseStatus
func (r *Request) StatusCode() int {
	if len(r.ResponseStatus) < 3 {
		return 0
	}
	code := 0
	for i := 0; i < 3 && i < len(r.ResponseStatus); i++ {
		if r.ResponseStatus[i] >= '0' && r.ResponseStatus[i] <= '9' {
			code = code*10 + int(r.ResponseStatus[i]-'0')
		}
	}
	return code
}

// DurationMs returns the duration in milliseconds
func (r *Request) DurationMs() float64 {
	return float64(r.Duration) / 1_000_000
}
