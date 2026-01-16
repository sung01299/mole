package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sung01299/mole/internal/ngrok"
	"github.com/sung01299/mole/internal/tui"
)

const version = "0.1.0"

func main() {
	// Check for version flag
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("mole %s\n", version)
		os.Exit(0)
	}

	// Initialize ngrok client
	baseURL := os.Getenv("NGROK_API_URL")
	if baseURL == "" {
		baseURL = ngrok.DefaultBaseURL
	}

	client := ngrok.NewClient(baseURL)

	// Check if ngrok is running
	if !client.IsAvailable() {
		fmt.Println("⚠️  Cannot connect to ngrok local API at", baseURL)
		fmt.Println()
		fmt.Println("Make sure ngrok is running:")
		fmt.Println("  $ ngrok http 8080")
		fmt.Println()
		fmt.Println("Or set NGROK_API_URL environment variable to a custom URL.")
		os.Exit(1)
	}

	// Create and run TUI
	app := tui.NewApp(client)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
