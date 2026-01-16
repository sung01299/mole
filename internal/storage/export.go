package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ExportSession represents a session for JSON export
type ExportSession struct {
	ID        string          `json:"id"`
	TunnelURL string          `json:"tunnel_url"`
	StartedAt time.Time       `json:"started_at"`
	EndedAt   *time.Time      `json:"ended_at,omitempty"`
	Requests  []ExportRequest `json:"requests"`
}

// ExportRequest represents a request for JSON export
type ExportRequest struct {
	ID         string              `json:"id"`
	Method     string              `json:"method"`
	Path       string              `json:"path"`
	StatusCode int                 `json:"status_code"`
	DurationMS int64               `json:"duration_ms"`
	Timestamp  time.Time           `json:"timestamp"`
	Request    ExportHTTPData      `json:"request"`
	Response   ExportHTTPData      `json:"response"`
	Starred    bool                `json:"starred"`
}

// ExportHTTPData represents HTTP data for export
type ExportHTTPData struct {
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"body"`
}

// ExportSessionToJSON exports a session to a JSON file
func (s *Storage) ExportSessionToJSON(sessionID string, outputPath string) error {
	// Get session info
	var sess Session
	var endedAt *time.Time
	err := s.db.QueryRow(
		"SELECT id, tunnel_url, started_at, ended_at FROM sessions WHERE id = ?",
		sessionID,
	).Scan(&sess.ID, &sess.TunnelURL, &sess.StartedAt, &endedAt)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	sess.EndedAt = endedAt

	// Get requests
	requests, err := s.GetSessionRequests(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get requests: %w", err)
	}

	// Build export structure
	export := ExportSession{
		ID:        sess.ID,
		TunnelURL: sess.TunnelURL,
		StartedAt: sess.StartedAt,
		EndedAt:   sess.EndedAt,
		Requests:  make([]ExportRequest, len(requests)),
	}

	for i, req := range requests {
		export.Requests[i] = ExportRequest{
			ID:         req.ID,
			Method:     req.Method,
			Path:       req.Path,
			StatusCode: req.StatusCode,
			DurationMS: req.DurationMS,
			Timestamp:  req.Timestamp,
			Request: ExportHTTPData{
				Headers: req.ReqHeaders,
				Body:    req.ReqBody,
			},
			Response: ExportHTTPData{
				Headers: req.ResHeaders,
				Body:    req.ResBody,
			},
			Starred: req.Starred,
		}
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Ensure directory exists
	if dir := filepath.Dir(outputPath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Write file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// ExportCurrentSession exports the current session to a JSON file
func (s *Storage) ExportCurrentSession(outputPath string) error {
	if s.sessionID == "" {
		return fmt.Errorf("no active session")
	}
	return s.ExportSessionToJSON(s.sessionID, outputPath)
}

// GenerateExportFilename generates a filename for export
func GenerateExportFilename() string {
	return fmt.Sprintf("mole_export_%s.json", time.Now().Format("2006-01-02_15-04-05"))
}

// ExportRequests exports specific requests to a JSON file
func (s *Storage) ExportRequests(requestIDs []string, outputPath string) error {
	if len(requestIDs) == 0 {
		return fmt.Errorf("no requests to export")
	}

	var requests []ExportRequest

	for _, id := range requestIDs {
		var req HistoryRequest
		var reqHeadersJSON, resHeadersJSON string

		err := s.db.QueryRow(`
			SELECT id, session_id, method, path, status_code, duration_ms, timestamp, 
			       req_headers, req_body, res_headers, res_body, starred
			FROM requests WHERE id = ?
		`, id).Scan(
			&req.ID, &req.SessionID, &req.Method, &req.Path, &req.StatusCode,
			&req.DurationMS, &req.Timestamp, &reqHeadersJSON, &req.ReqBody,
			&resHeadersJSON, &req.ResBody, &req.Starred,
		)
		if err != nil {
			continue // Skip not found
		}

		json.Unmarshal([]byte(reqHeadersJSON), &req.ReqHeaders)
		json.Unmarshal([]byte(resHeadersJSON), &req.ResHeaders)

		requests = append(requests, ExportRequest{
			ID:         req.ID,
			Method:     req.Method,
			Path:       req.Path,
			StatusCode: req.StatusCode,
			DurationMS: req.DurationMS,
			Timestamp:  req.Timestamp,
			Request: ExportHTTPData{
				Headers: req.ReqHeaders,
				Body:    req.ReqBody,
			},
			Response: ExportHTTPData{
				Headers: req.ResHeaders,
				Body:    req.ResBody,
			},
			Starred: req.Starred,
		})
	}

	data, err := json.MarshalIndent(requests, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
