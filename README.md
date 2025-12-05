# Hookshot

A self-hostable webhook relay for local development. Forward webhooks from external services to your localhost.

```
[Stripe] → [your-relay.fly.io/t/abc12345...] → [tunnel] → [localhost:3000/webhooks]
```

## Features

- **Single binary** - Easy to deploy and run
- **Persistent WebSocket connections** - Reliable forwarding
- **Unique endpoint URLs** - Each tunnel gets its own URL
- **Request history** - View recent webhooks
- **Replay webhooks** - Re-send failed requests for debugging
- **Interactive TUI** - Cute terminal UI with live request stream
- **Colorized logging** - See requests in real-time
- **Auth tokens** - Secure private relays
- **Config file support** - YAML configuration
- **Multiple targets** - Route by path to different local services
- **TLS support** - Native HTTPS for the server

## Installation

```bash
go install github.com/lance0/hookshot/cmd/hookshot@latest
```

Or build from source:

```bash
git clone https://github.com/lance0/hookshot.git
cd hookshot
go build -o hookshot ./cmd/hookshot
```

## Quick Start

### 1. Run the Server (on your VPS)

```bash
hookshot server --port 8080 --public-url https://relay.example.com
```

### 2. Run the Client (locally)

```bash
hookshot client --server https://relay.example.com --target http://localhost:3000
```

Output:
```
✓ Connected!

  Tunnel ID:  abc12345
  Public URL: https://relay.example.com/t/abc12345-...
  Forwarding: http://localhost:3000

  Waiting for requests...
──────────────────────────────────────────────────
[15:04:05] → POST    /webhooks/stripe (d08ba939)
[15:04:05] ← 200 (15ms)
```

### 3. Configure Your Webhook

Point your webhook provider to your public URL:
```
https://relay.example.com/t/{your-tunnel-id}/webhooks/stripe
```

## Commands

### `hookshot server`

Run the relay server.

```bash
hookshot server [flags]

Flags:
  -c, --config string     Config file path
  -p, --port int          Port to listen on (default 8080)
      --host string       Host to bind to (default "0.0.0.0")
      --public-url string Public URL for display
      --max-requests int  Max requests to store per tunnel (default 100)
      --token string      Auth token (required for client connections if set)
      --tls-cert string   Path to TLS certificate file
      --tls-key string    Path to TLS key file
```

### `hookshot client`

Connect to a relay server.

```bash
hookshot client [flags]

Flags:
  -c, --config string   Config file path
  -s, --server string   Server URL (required, or set in config)
  -t, --target string   Local target URL (default "http://localhost:3000")
      --id string       Requested tunnel ID (optional)
      --token string    Auth token for server
  -v, --verbose         Show request/response bodies
      --tui             Enable interactive TUI mode
```

## Interactive TUI Mode

Launch the client with `--tui` for an interactive terminal interface:

```bash
hookshot client --server https://relay.example.com --tui
```

```
┌────────────────────────────────────────────────────────────────────┐
│  hookshot                           tunnel: abc12345  ● connected  │
│  Public URL: https://relay.example.com/t/abc12345...               │
│  Forwarding: http://localhost:3000                                 │
├────────────────────────────────────────────────────────────────────┤
│  REQUESTS                                     [r]eplay  [/]filter  │
│  ────────────────────────────────────────────────────────────────  │
│  ▸ POST   /webhooks/stripe     200   12ms   just now     d08ba939  │
│    GET    /api/health          200    3ms   2s ago       f4a21c87  │
│    POST   /webhooks/github     500   45ms   5s ago       b7e93d12  │
├────────────────────────────────────────────────────────────────────┤
│  REQUEST DETAIL                                                    │
│  ────────────────────────────────────────────────────────────────  │
│  POST /webhooks/stripe                                             │
│  Content-Type: application/json                                    │
│  {"event":"payment.success","amount":1000}                         │
│  Response: 200 (12ms)                                              │
└────────────────────────────────────────────────────────────────────┘
  ↑↓ navigate  r replay  / filter  q quit
```

### TUI Keybindings

| Key | Action |
|-----|--------|
| `↑` / `k` | Move selection up |
| `↓` / `j` | Move selection down |
| `r` | Replay selected request |
| `/` | Start filter mode |
| `Esc` | Clear filter |
| `q` / `Ctrl+C` | Quit |

### `hookshot requests`

List recent requests for a tunnel.

```bash
hookshot requests --server https://relay.example.com --tunnel abc123
```

### `hookshot replay`

Replay a previous request.

```bash
hookshot replay --server https://relay.example.com --tunnel abc123 --request d08ba939
```

## Config File

Create `hookshot.yaml` in your current directory or `~/.config/hookshot/config.yaml`:

```yaml
# Server configuration
server:
  port: 8080
  host: 0.0.0.0
  public_url: https://relay.example.com
  token: your-secret-token
  # tls_cert: /path/to/cert.pem
  # tls_key: /path/to/key.pem

# Client configuration
client:
  server: https://relay.example.com
  tunnel_id: my-project
  token: your-secret-token
  verbose: false

  # Single target
  target: http://localhost:3000

  # OR multiple targets (route by path)
  # routes:
  #   - path: /api
  #     target: http://localhost:3000
  #   - path: /webhooks
  #     target: http://localhost:4000
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/t/{tunnel_id}/*` | ANY | Webhook receiver |
| `/ws` | WebSocket | Client connection |
| `/api/tunnels/{id}/requests` | GET | List recent requests |
| `/api/tunnels/{id}/requests/{req_id}/replay` | POST | Replay a request |
| `/health` | GET | Health check |

## License

MIT
