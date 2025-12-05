# Changelog

## [Unreleased]

### Added
- Interactive TUI mode (`--tui` flag) with bubbletea
- Live request stream display in TUI
- Request detail view with headers and body
- Replay requests directly from TUI (press `r`)
- Filter/search requests by path, method, or ID (press `/`)
- Catppuccin Mocha color theme (cute pastel colors)
- Auth tokens for private relays (`--token` flag)
- YAML config file support (`--config` or auto-discovered hookshot.yaml)
- Multiple local targets via route-based path matching
- HTTPS/TLS support for server (`--tls-cert`, `--tls-key`)
- Verbose mode for request/response body logging (`--verbose`)

## [0.1.0] - 2025-12-05

### Added
- Initial release of hookshot
- Relay server with WebSocket tunnel support
- Client with auto-reconnect and colorized logging
- Request history storage (last 100 per tunnel)
- Replay API for re-sending webhooks
- CLI commands: server, client, requests, replay
- Flexible tunnel IDs (client-requested or server-assigned)
