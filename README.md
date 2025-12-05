# Hookshot

A self-hostable webhook relay for local development. Forward webhooks from external services to your localhost.

```
[Stripe] → [your-relay.fly.io/t/abc123] → [tunnel] → [localhost:3000/webhooks]
```

## Features

- **Single binary** - Easy to deploy and run
- **Persistent WebSocket connections** - Reliable forwarding
- **Unique endpoint URLs** - Each tunnel gets its own URL
- **Request history** - View recent webhooks
- **Replay webhooks** - Re-send failed requests for debugging
- **Colorized logging** - See requests in real-time

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

  Tunnel ID:  abc123
  Public URL: https://relay.example.com/t/abc123
  Forwarding: http://localhost:3000

  Waiting for requests...
──────────────────────────────────────────────────
[15:04:05] → POST    /webhooks/stripe (d08ba939)
[15:04:05] ← 200 (15ms)
```

### 3. Configure Your Webhook

Point your webhook provider to your public URL:
```
https://relay.example.com/t/abc123/webhooks/stripe
```

## Commands

### `hookshot server`

Run the relay server.

```bash
hookshot server [flags]

Flags:
  -p, --port int          Port to listen on (default 8080)
      --host string       Host to bind to (default "0.0.0.0")
      --public-url string Public URL for display
      --max-requests int  Max requests to store per tunnel (default 100)
```

### `hookshot client`

Connect to a relay server.

```bash
hookshot client [flags]

Flags:
  -s, --server string  Server URL (required)
  -t, --target string  Local target URL (default "http://localhost:3000")
      --id string      Requested tunnel ID (optional)
```

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
