package util

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
)

// PrettyJSON formats JSON with indentation
func PrettyJSON(data string) string {
	if data == "" {
		return ""
	}

	var buf bytes.Buffer
	err := json.Indent(&buf, []byte(data), "", "  ")
	if err != nil {
		// Not valid JSON, return as-is
		return data
	}
	return buf.String()
}

// IsJSON checks if a string is valid JSON
func IsJSON(data string) bool {
	data = strings.TrimSpace(data)
	if data == "" {
		return false
	}
	return (data[0] == '{' || data[0] == '[') && json.Valid([]byte(data))
}

// HighlightJSON applies syntax highlighting to JSON
// Returns the highlighted string (with ANSI codes) or the original if highlighting fails
func HighlightJSON(data string) string {
	if data == "" {
		return ""
	}

	var buf bytes.Buffer
	err := quick.Highlight(&buf, data, "json", "terminal256", "monokai")
	if err != nil {
		return data
	}
	return buf.String()
}

// FormatBody formats request/response body
// If it's JSON, it will be pretty-printed and highlighted
func FormatBody(body string, contentType string) string {
	if body == "" {
		return "(empty)"
	}

	// Check if it's JSON based on content type or content
	isJSON := strings.Contains(contentType, "application/json") || IsJSON(body)

	if isJSON {
		pretty := PrettyJSON(body)
		return HighlightJSON(pretty)
	}

	return body
}

// TruncateString truncates a string to maxLen characters
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
