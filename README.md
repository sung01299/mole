# 🕳 Mole

> The missing TUI for ngrok

Mole is a terminal-based user interface for monitoring and debugging ngrok traffic. Stop context-switching to your browser — inspect webhooks, replay requests, and debug APIs without leaving your terminal.

## Features

- **Real-time traffic monitoring** — Watch HTTP requests flow through your ngrok tunnel
- **Request inspection** — View headers and body with JSON syntax highlighting
- **Replay requests** — Re-send any captured request with a single keystroke
- **Vim-style navigation** — Navigate with `j`/`k`, `g`/`G`, and other familiar keybindings
- **Responsive layout** — Adapts to your terminal size automatically

## Installation

### From Source

```bash
go install github.com/mole-cli/mole@latest
```

### Using Homebrew (coming soon)

```bash
brew install mole-cli/tap/mole
```

## Usage

1. Start ngrok in one terminal:

```bash
ngrok http 8080
```

2. Run mole in another terminal:

```bash
mole
```

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `g` | Go to first item |
| `G` | Go to last item |
| `Enter` | Expand detail view |
| `Esc` | Back to list |
| `r` | Replay selected request |
| `q` | Quit |

## Configuration

Mole connects to ngrok's local API at `http://127.0.0.1:4040` by default. You can override this with the `NGROK_API_URL` environment variable:

```bash
NGROK_API_URL=http://localhost:4041 mole
```

## Roadmap

- [ ] Search/filter requests by path or status code
- [ ] Copy request as cURL command
- [ ] Support for Cloudflare Tunnel
- [ ] Custom themes and keybindings

## License

MIT
