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

## Phase 2: Production Features

- [ ] Auth tokens for private relays
- [ ] Multiple local targets (route by path)
- [ ] Config file support
- [ ] HTTPS/TLS support for server
- [ ] Request/response body logging (truncated)
- [ ] Timing information in logs

## Phase 3: TUI & Polish

- [ ] Request inspection TUI (bubbletea)
- [ ] Live request stream in TUI
- [ ] Request detail view
- [ ] Replay from TUI
- [ ] Search/filter requests
- [ ] Full-text search in bodies

## Future Ideas

- [ ] Persistent storage (SQLite) for request history
- [ ] Web dashboard for request inspection
- [ ] Metrics/stats endpoint
- [ ] Rate limiting
- [ ] Multiple tunnels per client
- [ ] Fly.io one-click deploy template
