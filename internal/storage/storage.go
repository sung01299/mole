package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Storage handles persistent storage of request history
type Storage struct {
	db        *sql.DB
	sessionID string
}

// Session represents a mole session (one ngrok connection)
type Session struct {
	ID        string
	TunnelURL string
	StartedAt time.Time
	EndedAt   *time.Time
}

// HistoryRequest represents a stored request
type HistoryRequest struct {
	ID          string
	SessionID   string
	Method      string
	Path        string
	StatusCode  int
	DurationMS  int64
	Timestamp   time.Time
	ReqHeaders  map[string][]string
	ReqBody     string
	ResHeaders  map[string][]string
	ResBody     string
	Starred     bool
}

// New creates a new Storage instance
func New() (*Storage, error) {
	dbPath, err := getDBPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get db path: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	s := &Storage{db: db}

	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	return s, nil
}

// getDBPath returns the path to the SQLite database
func getDBPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".mole", "history.db"), nil
}

// initSchema creates the database tables if they don't exist
func (s *Storage) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		tunnel_url TEXT,
		started_at DATETIME,
		ended_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS requests (
		id TEXT PRIMARY KEY,
		session_id TEXT,
		method TEXT,
		path TEXT,
		status_code INTEGER,
		duration_ms INTEGER,
		timestamp DATETIME,
		req_headers TEXT,
		req_body TEXT,
		res_headers TEXT,
		res_body TEXT,
		starred BOOLEAN DEFAULT FALSE,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE INDEX IF NOT EXISTS idx_requests_session ON requests(session_id);
	CREATE INDEX IF NOT EXISTS idx_requests_timestamp ON requests(timestamp);
	CREATE INDEX IF NOT EXISTS idx_requests_starred ON requests(starred);
	`

	_, err := s.db.Exec(schema)
	return err
}

// StartSession creates a new session and returns its ID
func (s *Storage) StartSession(tunnelURL string) (string, error) {
	id := fmt.Sprintf("session_%d", time.Now().UnixNano())
	s.sessionID = id

	_, err := s.db.Exec(
		"INSERT INTO sessions (id, tunnel_url, started_at) VALUES (?, ?, ?)",
		id, tunnelURL, time.Now(),
	)
	if err != nil {
		return "", err
	}

	return id, nil
}

// EndSession marks the current session as ended
func (s *Storage) EndSession() error {
	if s.sessionID == "" {
		return nil
	}

	_, err := s.db.Exec(
		"UPDATE sessions SET ended_at = ? WHERE id = ?",
		time.Now(), s.sessionID,
	)
	return err
}

// SaveRequest saves a request to the database
func (s *Storage) SaveRequest(req HistoryRequest) error {
	if s.sessionID == "" {
		return fmt.Errorf("no active session")
	}

	reqHeaders, _ := json.Marshal(req.ReqHeaders)
	resHeaders, _ := json.Marshal(req.ResHeaders)

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO requests 
		(id, session_id, method, path, status_code, duration_ms, timestamp, req_headers, req_body, res_headers, res_body, starred)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		req.ID, s.sessionID, req.Method, req.Path, req.StatusCode, req.DurationMS,
		req.Timestamp, string(reqHeaders), req.ReqBody, string(resHeaders), req.ResBody, req.Starred,
	)
	return err
}

// ToggleStar toggles the starred status of a request
func (s *Storage) ToggleStar(requestID string) (bool, error) {
	// Get current starred status
	var starred bool
	err := s.db.QueryRow("SELECT starred FROM requests WHERE id = ?", requestID).Scan(&starred)
	if err != nil {
		return false, err
	}

	// Toggle it
	newStarred := !starred
	_, err = s.db.Exec("UPDATE requests SET starred = ? WHERE id = ?", newStarred, requestID)
	if err != nil {
		return false, err
	}

	return newStarred, nil
}

// IsStarred checks if a request is starred
func (s *Storage) IsStarred(requestID string) bool {
	var starred bool
	err := s.db.QueryRow("SELECT starred FROM requests WHERE id = ?", requestID).Scan(&starred)
	if err != nil {
		return false
	}
	return starred
}

// GetSessions returns all sessions, ordered by start time descending
func (s *Storage) GetSessions() ([]Session, error) {
	rows, err := s.db.Query(`
		SELECT id, tunnel_url, started_at, ended_at 
		FROM sessions 
		ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var sess Session
		var endedAt sql.NullTime
		if err := rows.Scan(&sess.ID, &sess.TunnelURL, &sess.StartedAt, &endedAt); err != nil {
			return nil, err
		}
		if endedAt.Valid {
			sess.EndedAt = &endedAt.Time
		}
		sessions = append(sessions, sess)
	}

	return sessions, nil
}

// GetSessionRequests returns all requests for a session
func (s *Storage) GetSessionRequests(sessionID string) ([]HistoryRequest, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, method, path, status_code, duration_ms, timestamp, 
		       req_headers, req_body, res_headers, res_body, starred
		FROM requests 
		WHERE session_id = ?
		ORDER BY timestamp DESC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanRequests(rows)
}

// GetStarredRequests returns all starred requests
func (s *Storage) GetStarredRequests() ([]HistoryRequest, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, method, path, status_code, duration_ms, timestamp, 
		       req_headers, req_body, res_headers, res_body, starred
		FROM requests 
		WHERE starred = TRUE
		ORDER BY timestamp DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanRequests(rows)
}

// SearchRequests searches requests by path or method
func (s *Storage) SearchRequests(query string) ([]HistoryRequest, error) {
	searchTerm := "%" + query + "%"
	rows, err := s.db.Query(`
		SELECT id, session_id, method, path, status_code, duration_ms, timestamp, 
		       req_headers, req_body, res_headers, res_body, starred
		FROM requests 
		WHERE path LIKE ? OR method LIKE ? OR req_body LIKE ? OR res_body LIKE ?
		ORDER BY timestamp DESC
		LIMIT 100
	`, searchTerm, searchTerm, searchTerm, searchTerm)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanRequests(rows)
}

// GetRecentRequests returns recent requests across all sessions
func (s *Storage) GetRecentRequests(limit int) ([]HistoryRequest, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, method, path, status_code, duration_ms, timestamp, 
		       req_headers, req_body, res_headers, res_body, starred
		FROM requests 
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanRequests(rows)
}

func (s *Storage) scanRequests(rows *sql.Rows) ([]HistoryRequest, error) {
	var requests []HistoryRequest
	for rows.Next() {
		var req HistoryRequest
		var reqHeadersJSON, resHeadersJSON string

		if err := rows.Scan(
			&req.ID, &req.SessionID, &req.Method, &req.Path, &req.StatusCode,
			&req.DurationMS, &req.Timestamp, &reqHeadersJSON, &req.ReqBody,
			&resHeadersJSON, &req.ResBody, &req.Starred,
		); err != nil {
			return nil, err
		}

		json.Unmarshal([]byte(reqHeadersJSON), &req.ReqHeaders)
		json.Unmarshal([]byte(resHeadersJSON), &req.ResHeaders)

		requests = append(requests, req)
	}

	return requests, nil
}

// DeleteRequest deletes a request by ID
func (s *Storage) DeleteRequest(requestID string) error {
	_, err := s.db.Exec("DELETE FROM requests WHERE id = ?", requestID)
	return err
}

// DeleteSession deletes a session and all its requests
func (s *Storage) DeleteSession(sessionID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec("DELETE FROM requests WHERE session_id = ?", sessionID); err != nil {
		tx.Rollback()
		return err
	}

	if _, err := tx.Exec("DELETE FROM sessions WHERE id = ?", sessionID); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// Cleanup removes old requests (keeps starred and recent)
func (s *Storage) Cleanup(keepDays int, keepCount int) error {
	cutoff := time.Now().AddDate(0, 0, -keepDays)

	// Delete old non-starred requests, keeping at least keepCount
	_, err := s.db.Exec(`
		DELETE FROM requests 
		WHERE starred = FALSE 
		AND timestamp < ?
		AND id NOT IN (
			SELECT id FROM requests 
			WHERE starred = FALSE 
			ORDER BY timestamp DESC 
			LIMIT ?
		)
	`, cutoff, keepCount)

	if err != nil {
		return err
	}

	// Delete empty sessions
	_, err = s.db.Exec(`
		DELETE FROM sessions 
		WHERE id NOT IN (SELECT DISTINCT session_id FROM requests)
	`)

	return err
}

// GetStats returns storage statistics
func (s *Storage) GetStats() (sessionCount, requestCount, starredCount int, err error) {
	err = s.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessionCount)
	if err != nil {
		return
	}
	err = s.db.QueryRow("SELECT COUNT(*) FROM requests").Scan(&requestCount)
	if err != nil {
		return
	}
	err = s.db.QueryRow("SELECT COUNT(*) FROM requests WHERE starred = TRUE").Scan(&starredCount)
	return
}

// Close closes the database connection
func (s *Storage) Close() error {
	if s.sessionID != "" {
		s.EndSession()
	}
	return s.db.Close()
}

// CurrentSessionID returns the current session ID
func (s *Storage) CurrentSessionID() string {
	return s.sessionID
}
