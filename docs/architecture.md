# Architecture

## System diagram

```
                  ┌──────────────────────────┐
                  │   Citadel Server         │
                  │   name: "Throne"         │
                  │                          │
                  │  • UDP directed broadcast │
                  │     (port 7778, 2 s)     │
                  │  • mDNS advertise        │
                  │     _citadel._tcp        │
                  │  • TCP listener (:7777)  │
                  │  • Hub goroutine         │
                  │     ─ client registry    │
                  │     ─ name uniqueness    │
                  │     ─ broadcast / kick   │
                  │  • Activity log (jsonl)  │
                  │  • Admin TUI (Bubble Tea)│
                  └────────────┬─────────────┘
                               │ length-prefixed JSON frames
              ┌────────────────┼────────────────┐
       ┌──────▼─────┐   ┌──────▼─────┐   ┌──────▼─────┐
       │ Client A   │   │ Client B   │   │ Client C   │
       │ "Vipul"    │   │ "Aarav"    │   │ "Maya"     │
       │ split-pane │   │            │   │            │
       │ TUI        │   │            │   │            │
       └────────────┘   └────────────┘   └────────────┘
```

## Components

### `internal/proto/`
Wire protocol: length-prefixed JSON frames and the `Envelope` type. Every message on the wire is an `Envelope` with a typed `Payload`. See [protocol.md](protocol.md).

### `internal/discovery/`
`advertise.go` — registers the server via mDNS (`_citadel._tcp`).  
`broadcast.go` — server sends UDP directed-broadcast presence packets every 2 s; client listens on port 7778.  
`browse.go` — discovers servers via mDNS, returning a channel of `ServerInfo`.  
See [discovery.md](discovery.md).

### `internal/server/`
- `server.go` — TCP listener, SIGINT shutdown, mDNS + UDP broadcast lifetime.
- `hub.go` — owns the `name → *clientConn` map via a goroutine + channels (no mutexes on shared state). Handles register/unregister/broadcast/direct/kick.
- `client.go` — one goroutine reads frames from the socket, another drains the outbound queue (64 messages; overflow kicks the client).
- `log.go` — JSON-lines activity logger to `~/.citadel/<name>/activity.log`.
- `tui.go` — Bubble Tea admin interface: status bar, client list, activity log pane, command input.

### `internal/client/`
- `conn.go` — dial, hello/welcome handshake, frame read/write goroutines.
- `commands.go` — slash command parser (`/who`, `/msg`, `/quit`, `/help`).
- `tui.go` — Bubble Tea client: discovery view → name-prompt view → split-pane chat view (game pane hidden until Phase 6).

### `cmd/citadel/`
Single binary. Subcommand dispatcher: `server | client | version`.

## Data flow (client connect + chat)

```
Client                         Server
  │                               │
  ├─ TCP dial ────────────────────▶│
  ├─ hello{name,version} ─────────▶│
  │                               ├─ validate name (unique, charset)
  │◀─ welcome{server_name,peers} ──┤
  │                               │
  ├─ [heartbeat loop]             │
  ├─ ping ────────────────────────▶│
  │◀─ pong ─────────────────────── ┤
  │                               │
  ├─ chat{text} ──────────────────▶│
  │                               ├─ broadcast to all clients
  │◀─ chat{from:"Vipul",text} ─────┤ (echo to sender too)
  │                               │
  ├─ leave ───────────────────────▶│
  │                               ├─ broadcast system{event:leave}
  │◀─ TCP close ─────────────────── ┤
```
