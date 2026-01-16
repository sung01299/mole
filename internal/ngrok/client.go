package ngrok

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	DefaultBaseURL = "http://127.0.0.1:4040"
	DefaultTimeout = 5 * time.Second
)

// Client is an HTTP client for the ngrok local API
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new ngrok API client
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// get performs a GET request and decodes the JSON response
func (c *Client) get(path string, result interface{}) error {
	url := c.baseURL + path
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: status %d: %s", path, resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("GET %s: decode error: %w", path, err)
	}

	return nil
}

// post performs a POST request with optional JSON body
func (c *Client) post(path string, body io.Reader) error {
	url := c.baseURL + path

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	// Accept 200, 201, 204 as success
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("POST %s: status %d: %s", path, resp.StatusCode, string(respBody))
}

// IsAvailable checks if the ngrok API is reachable
func (c *Client) IsAvailable() bool {
	url := c.baseURL + "/api"
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
