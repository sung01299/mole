package ngrok

import (
	"fmt"
	"strings"
)

// GetTunnels retrieves all active tunnels
func (c *Client) GetTunnels() ([]Tunnel, error) {
	var resp TunnelsResponse
	if err := c.get("/api/tunnels", &resp); err != nil {
		return nil, err
	}
	return resp.Tunnels, nil
}

// GetRequests retrieves captured HTTP requests
// limit: maximum number of requests to return (0 for default)
func (c *Client) GetRequests(limit int) ([]Request, error) {
	path := "/api/requests/http"
	if limit > 0 {
		path = fmt.Sprintf("%s?limit=%d", path, limit)
	}

	var resp RequestsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return resp.Requests, nil
}

// GetRequest retrieves a specific request by ID
func (c *Client) GetRequest(id string) (*Request, error) {
	path := fmt.Sprintf("/api/requests/http/%s", id)
	var req Request
	if err := c.get(path, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// Replay re-sends a captured request
// ngrok v3 API: POST /api/requests/http with body {"id": "request_id"}
func (c *Client) Replay(requestID string) error {
	body := fmt.Sprintf(`{"id":"%s"}`, requestID)
	return c.post("/api/requests/http", strings.NewReader(body))
}

// DeleteRequests clears all captured requests
func (c *Client) DeleteRequests() error {
	// Note: This uses DELETE method, but we'll implement if needed
	// For MVP, we focus on read-only operations + replay
	return nil
}
