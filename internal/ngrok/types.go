package ngrok

import (
	"encoding/base64"
	"strings"
	"time"
)

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
	Method     string              `json:"method,omitempty"`
	Proto      string              `json:"proto"`
	Headers    map[string][]string `json:"headers"`
	URI        string              `json:"uri,omitempty"`
	Raw        string              `json:"raw"`
	StatusCode int                 `json:"status_code,omitempty"` // For response
	Status     string              `json:"status,omitempty"`      // e.g., "200 OK"
}

// DecodeBody decodes the base64-encoded raw field and extracts the body
func (h *HTTPData) DecodeBody() string {
	if h.Raw == "" {
		return ""
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(h.Raw)
	if err != nil {
		// Try URL-safe base64
		decoded, err = base64.URLEncoding.DecodeString(h.Raw)
		if err != nil {
			return h.Raw // Return as-is if not base64
		}
	}

	raw := string(decoded)

	// The raw contains full HTTP message (headers + body)
	// Find the empty line that separates headers from body
	headerEnd := strings.Index(raw, "\r\n\r\n")
	if headerEnd == -1 {
		headerEnd = strings.Index(raw, "\n\n")
		if headerEnd == -1 {
			return raw // No body separator found, return full content
		}
		return strings.TrimSpace(raw[headerEnd+2:])
	}

	return strings.TrimSpace(raw[headerEnd+4:])
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

// StatusCode extracts the numeric status code
func (r *Request) StatusCode() int {
	// Try Response.StatusCode first (ngrok API v2)
	if r.Response.StatusCode > 0 {
		return r.Response.StatusCode
	}

	// Try parsing from ResponseStatus (e.g., "200 OK")
	if len(r.ResponseStatus) >= 3 {
		code := 0
		for i := 0; i < 3 && i < len(r.ResponseStatus); i++ {
			if r.ResponseStatus[i] >= '0' && r.ResponseStatus[i] <= '9' {
				code = code*10 + int(r.ResponseStatus[i]-'0')
			}
		}
		if code > 0 {
			return code
		}
	}

	// Try parsing from Response.Status (e.g., "200 OK")
	if len(r.Response.Status) >= 3 {
		code := 0
		for i := 0; i < 3 && i < len(r.Response.Status); i++ {
			if r.Response.Status[i] >= '0' && r.Response.Status[i] <= '9' {
				code = code*10 + int(r.Response.Status[i]-'0')
			}
		}
		if code > 0 {
			return code
		}
	}

	// Try to extract from raw response (first line like "HTTP/1.1 200 OK")
	if r.Response.Raw != "" {
		decoded, err := base64.StdEncoding.DecodeString(r.Response.Raw)
		if err == nil {
			line := string(decoded)
			if idx := strings.Index(line, "\n"); idx > 0 {
				line = line[:idx]
			}
			// Parse "HTTP/1.1 200 OK"
			parts := strings.SplitN(line, " ", 3)
			if len(parts) >= 2 {
				code := 0
				for _, c := range parts[1] {
					if c >= '0' && c <= '9' {
						code = code*10 + int(c-'0')
					}
				}
				if code > 0 {
					return code
				}
			}
		}
	}

	return 0
}

// DurationMs returns the duration in milliseconds
func (r *Request) DurationMs() float64 {
	return float64(r.Duration) / 1_000_000
}

// ResponseSize returns the size of the response body in bytes
func (r *Request) ResponseSize() int {
	body := r.Response.DecodeBody()
	return len(body)
}
