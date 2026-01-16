package tui

import "github.com/charmbracelet/lipgloss"

// Color palette
var (
	ColorPrimary   = lipgloss.Color("#7C3AED") // Purple
	ColorSecondary = lipgloss.Color("#10B981") // Green
	ColorWarning   = lipgloss.Color("#F59E0B") // Amber
	ColorError     = lipgloss.Color("#EF4444") // Red
	ColorMuted     = lipgloss.Color("#6B7280") // Gray
	ColorBorder    = lipgloss.Color("#374151") // Dark Gray
	ColorHighlight = lipgloss.Color("#1F2937") // Darker Gray for backgrounds
)

// Status code colors
func StatusCodeColor(code int) lipgloss.Color {
	switch {
	case code >= 200 && code < 300:
		return ColorSecondary // Green for success
	case code >= 300 && code < 400:
		return lipgloss.Color("#3B82F6") // Blue for redirects
	case code >= 400 && code < 500:
		return ColorWarning // Amber for client errors
	case code >= 500:
		return ColorError // Red for server errors
	default:
		return ColorMuted
	}
}

// HTTP method colors
func MethodColor(method string) lipgloss.Color {
	switch method {
	case "GET":
		return lipgloss.Color("#3B82F6") // Blue
	case "POST":
		return ColorSecondary // Green
	case "PUT", "PATCH":
		return ColorWarning // Amber
	case "DELETE":
		return ColorError // Red
	default:
		return ColorMuted
	}
}

// Styles
var (
	// Base styles
	BaseStyle = lipgloss.NewStyle()

	// Header styles
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorPrimary).
			Padding(0, 1)

	TunnelURLStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	TunnelLocalStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	// List styles
	ListTitleStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Padding(0, 1)

	SelectedItemStyle = lipgloss.NewStyle().
				Background(ColorHighlight).
				Foreground(lipgloss.Color("#FFFFFF"))

	NormalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D1D5DB"))

	// Method badge style
	MethodStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)

	// Status code style
	StatusStyle = lipgloss.NewStyle().
			Bold(true)

	// Path style
	PathStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D1D5DB"))

	// Duration style
	DurationStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Detail panel styles
	DetailTitleStyle = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true).
				Padding(0, 1)

	DetailLabelStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	DetailValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF"))

	// Border styles
	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder)

	ActiveBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPrimary)

	// Help/Footer styles
	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	// Error style
	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	// Spinner style
	SpinnerStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary)
)
