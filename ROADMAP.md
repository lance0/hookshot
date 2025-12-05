# Hookshot Roadmap

## Phase 1: Core + Replay (Complete)

- [x] Project setup with Go module
- [x] WebSocket-based tunnel protocol
- [x] Relay server with webhook forwarding
- [x] Client with auto-reconnect
- [x] Request history storage
- [x] Replay API
- [x] Colorized console logging
- [x] CLI with server, client, requests, replay commands

## Phase 2: Production Features (Complete)

- [x] Auth tokens for private relays
- [x] Multiple local targets (route by path)
- [x] Config file support (YAML)
- [x] HTTPS/TLS support for server
- [x] Request/response body logging (truncated, via --verbose)
- [x] Timing information in logs

## Phase 3: TUI & Polish (Complete)

- [x] Request inspection TUI (bubbletea)
- [x] Live request stream in TUI
- [x] Request detail view
- [x] Cute Catppuccin Mocha theme
- [x] Replay from TUI
- [x] Search/filter requests

## Phase 4: Security & Reliability Hardening (Complete)

- [x] Full UUID tunnel IDs (36 chars internally, 8-char display)
- [x] Server-generated IDs only (ignore client requests)
- [x] Replay API tunnel ownership verification
- [x] Request body size limits (configurable, default 10MB)
- [x] WebSocket message size limits
- [x] WebSocket origin validation
- [x] Bearer token auth only (removed query string tokens)
- [x] Graceful server shutdown
- [x] Channel close race condition fix
- [x] Thread-safe WebSocket writes
- [x] Per-connection goroutine lifecycle
- [x] Proper URL building
- [x] Config file validation
- [x] Error context in logs

## Future Ideas

- [ ] Persistent storage (SQLite) for request history
- [ ] Web dashboard for request inspection
- [ ] Metrics/stats endpoint
- [ ] Rate limiting
- [ ] Multiple tunnels per client
- [ ] Fly.io one-click deploy template
