# Feature Backlog

Items here are decided/wanted but not yet implemented. When a feature lands, move it to its owning doc page and remove it here.

## Connection / presence

- `/me <action>` — emote (e.g. `* Vipul waves`); render-only, no protocol change needed
- `/nick <new-name>` — rename in-session; uniqueness re-check + broadcast rename; needs careful race handling
- Reconnect with exponential backoff after transient network failure
- Sticky names (server issues an auto-rejoin token stored locally; client sends it in `hello`)
- Spectator / read-only mode (connect without a handle; observes chat/game but cannot send)

## Server admin

- `/ban <name>` — temp IP block; needs an in-memory blocklist + unban command + TTL
- `/list` — text-only dump of connected clients (mostly redundant with server TUI)
- Server config file (TOML) once flag count > ~5
- Log rotation (`github.com/natefinch/roll` or `lumberjack`) when sessions run long

## Game-mode hooks

- `kind` taxonomy inside `game` payload: `move`, `action`, `state`, `lobby`, `tick`
- Game session lifecycle: lobby → start → in-progress → end (host-initiated via server command)
- Server-side game module (`internal/game/`) that opts in to inspect game payloads and validate/relay state
- Replay log: append every `Type:"game"` envelope verbatim to a separate file
- RTT indicator in client status bar (Phase 6 demo via `kind:"ping"` round-trips)

## UX / TUI

- Mode toggle hotkey: switch input focus between chat and game pane
- Color-by-name in chat (deterministic color from name hash)
- Mouse selection / copy in Bubble Tea (enable mouse mode)
- `PgUp`/`PgDn` scroll in chat viewport

## Quality / hardening (post-MVP)

- Shared passcode auth: server generates passcode at startup; client sends it in `hello` — drop-in upgrade from open model
- TLS + TOFU: server generates self-signed cert on first run; clients pin fingerprint on first connect
- msgpack / protobuf swap if game payloads get heavy
- Bots / fake clients for load testing the hub
- File transfer for game assets (chunked over `game` payload)
- Cross-compile + release via GoReleaser
