# Architecture

## System diagram

```
 ┌──────────────────────────────────────────────────────────────────────┐
 │  citadel host  (optional supervisor — not a citadel process itself)  │
 │  • spawns server --headless + connect --headless  •  Setpgid/child  │
 │  • monitors children; exits when either child exits                  │
 │  • ~/.citadel/host/current.json  (both pids + control socket paths)  │
 └─────────────────────────┬──────────────────────────┬────────────────┘
                    spawn  │                          │  spawn
                           ▼                          ▼
 ┌─────────────────────────────────┐  ┌─────────────────────────────────┐
 │  PROCESS: citadel server        │  │  PROCESS: citadel connect       │
 │                                 │  │                                 │
 │  TCP listener                   │  │  discovery:                     │
 │  mDNS advertise                 │  │  mDNS / UDP broadcast /         │
 │  UDP directed broadcast         │  │  localhost probe / --server     │
 │                                 │  │                                 │
 │  TCP conn ──▶ Hub               │  │  Conn  (TCP read+write)         │
 │  Hub: broadcast/kick/motd/log   │  │                                 │
 │                                 │◀─┼──── TCP frames ────────────────▶│
 │  HubEvent (lossy non-blocking): │  │  (length-prefixed JSON)         │
 │  ├─ Hub.Events                  │  │                                 │
 │  │   └──▶ Admin TUI             │  │  Envelope delivery:             │
 │  │        (TUI mode only;       │  │  ├─ conn.recvCh                 │
 │  │         omitted: headless)   │  │  │   └──▶ Chat TUI              │
 │  └─ Hub.ControlEvents           │  │  │        (TUI mode only;       │
 │      └──▶ ctrl bridge           │  │  │         omitted: headless)   │
 │            └──▶ control.Hub     │  │  └─ conn.ctrlCh  (lossy tap)   │
 │                  ring(200)      │  │      └──▶ ctrl bridge           │
 │                  + fanout       │  │            └──▶ control.Hub     │
 │            └──▶ UDS listener    │  │                  ring(200)      │
 │                 <pid>.sock      │  │                  + fanout       │
 │                 <pid>.json      │  │            └──▶ UDS listener    │
 └─────────────────────────────────┘  │                 <pid>.sock      │
                                      │                 <pid>.json      │
                                      └─────────────────────────────────┘
         │ UDS (JSON ops + events)             │ UDS (JSON ops + events)
         │ subscribe · list-peers              │ subscribe · list-peers
         │ kick · say · set-motd               │ send-chat · send-game
         │ shutdown · …                        │ shutdown · …
         │                                     │
         └──────────────────────┬──────────────┘
                                ▼
 ┌──────────────────────────────────────────────────────────────────────┐
 │  citadel dashboard  (or  citadel test)                               │
 │                                                                      │
 │  • ScanSentinels: polls ~/.citadel/run/*.json every 2 s             │
 │  • dials each <pid>.sock; subscribes (ring replay + live stream)     │
 │  • receives: peer-join/leave · kick · chat · motd-changed · …       │
 │  • sends:    shutdown (kill action from table view only)             │
 │  • dashboard: live process table  +  [Enter] drill-in per process   │
 │      server → read-only spectator Admin TUI  (HubEventSource/UDS)   │
 │      client → read-only spectator Chat TUI   (ConnController/UDS)   │
 └──────────────────────────────────────────────────────────────────────┘
```

The TCP and UDS channels are fully independent. TCP carries the chat/game protocol between server and clients. The UDS control socket is an always-on sidecar inside every process — it exposes ring-buffered event replay, live streaming, and action ops to external tooling without touching the TCP path.

## Components

### `internal/proto/`
Wire protocol: length-prefixed JSON frames and the `Envelope` type. Every message on the wire is an `Envelope` with a typed `Payload`. See [protocol.md](protocol.md).

### `internal/discovery/`
`advertise.go` — registers the server via mDNS (`_citadel._tcp`).  
`broadcast.go` — server sends UDP directed-broadcast presence packets every 2 s; client listens on port 7778.  
`browse.go` — discovers servers via mDNS, returning a channel of `ServerInfo`.  
See [discovery.md](discovery.md).

### `internal/control/`
Control plane infrastructure shared by server and client.
- `plane.go` — `Plane`: opens the UDS socket + writes/removes the sentinel file. Created in every process.
- `hub.go` — ring-buffer fan-out hub: 200-entry replay ring, subscriber management, two-stage delivery to avoid holding `ring.mu` during channel writes.
- `listener.go` — UDS accept loop; one `serveAttacher` goroutine per connection that handles subscribe/emit/action ops.
- `sentinel.go` — writes `~/.citadel/run/<pid>.json` on startup, removes it on shutdown.
- `scanner.go` — `ScanSentinels`, `WaitForSentinel` (used by `citadel host` and integration tests).
- `pointer.go` — `WriteClientPointer` / `WriteHostPointer` for session manifest files.
- `actions.go` — `ActionsProvider` interface (implemented by server and client packages; no import cycle).
- `client/conn.go` — minimal UDS client: dial, send JSON ops, receive event frames. Used by `citadel test`, `citadel dashboard`, and integration tests.

### `internal/server/`
- `server.go` — TCP listener, SIGINT shutdown, mDNS + UDP broadcast lifetime. Creates a `control.Plane` after bind; exposes `WaitListenAddr` so the command layer can retrieve the actual port when `--port 0` is used.
- `hub.go` — owns the `name → *clientConn` map via a goroutine + channels (no mutexes on shared state). Handles register/unregister/broadcast/direct/kick. Fans out `HubEvent` to both TUI (`Events`) and control-plane bridge (`ControlEvents`).
- `client.go` — one goroutine reads frames from the socket, another drains the outbound queue (64 messages; overflow kicks the client).
- `log.go` — JSON-lines activity logger to `~/.citadel/<name>/activity.log`.
- `tui.go` — Bubble Tea admin interface: status bar, client list, activity log pane, command input. Backed by `HubEventSource` (in-process or remote via control socket). Remote (dashboard drill-in) instances are **read-only**: input is disabled and replaced with a spectator indicator.
- `source.go` — `HubEventSource` interface + `inProcessSource` (wraps `*Hub`) + `remoteSource` (dials control socket; used by dashboard drill-in).
- `actions.go` — `serverActions`: `ActionsProvider` implementation wrapping `*Hub`.

### `internal/client/`
- `conn.go` — dial, hello/welcome handshake, frame read/write goroutines. Creates a `control.Plane` after handshake; fans out received envelopes to the control bridge.
- `commands.go` — slash command parser (`/who`, `/msg`, `/quit`, `/help`).
- `tui.go` — Bubble Tea client: discovery view → name-prompt view → split-pane chat view. Backed by `ConnController` (in-process or remote via control socket). Remote (dashboard drill-in) instances are **read-only**: input is disabled and replaced with a spectator indicator.
- `source.go` — `ConnController` interface + `inProcessController` (wraps `*Conn`) + `remoteConnController` (dials control socket; used by dashboard drill-in).
- `actions.go` — `connActions`: `ActionsProvider` implementation wrapping `*Conn`.

### `internal/dashboard/`
- `model.go` — `DashboardModel`: scans `~/.citadel/run/` every 2 s, shows a live table of running processes, supports [Enter] drill-in to a **read-only spectator** view of any server or client TUI. Peer count uses `ev:"peers"` as the authoritative snapshot and only applies incremental `peer-join`/`peer-leave` after `ev:"live"` (avoids ring-buffer replay overcounting).
- `actions.go` — spawn/kill/restart helpers: `spawnDetached` (uses `os.Executable()` + `Setpgid:true`), `startKill` (shutdown op + 3 s SIGTERM fallback), `doRestart`.

### `cmd/citadel/`
Single binary. Subcommand dispatcher: `server | connect | host | dashboard | test | update | version`.

## Run-state files

Every citadel process writes a sentinel and opens a control socket on startup:

```
~/.citadel/run/<pid>.json   sentinel: role, name, addr, socket path, args, started time
~/.citadel/run/<pid>.sock   Unix domain socket for the control plane (mode 0600)
~/.citadel/host/current.json    written by `citadel host` (both server + client pids/sockets)
~/.citadel/client/current.json  written by `citadel connect --headless`
```

All files are removed on clean shutdown. The dashboard and `citadel test` use these to find live processes. See [control.md](control.md), [dashboard.md](dashboard.md), and ADRs [0005](decisions/0005-control-plane-unix-socket.md)–[0007](decisions/0007-always-on-control-plane.md).

## Data flow

### TCP: client connect + chat

```
Client                               Server
  │                                     │
  ├─ TCP dial ──────────────────────────▶│
  ├─ hello{name,version} ───────────────▶│
  │                                     ├─ validate name (unique, charset)
  │◀─ welcome{server_name,motd,peers} ───┤
  │                                     │
  ├─ [client opens UDS control socket]  ├─ [server control socket already open]
  │   ~/.citadel/run/<cpid>.sock         │   ~/.citadel/run/<spid>.sock
  │   ~/.citadel/client/current.json     │   emits "peer-join" to control hub
  │   (headless only)                    │
  │                                     │
  ├─ [heartbeat loop]                   │
  ├─ ping ──────────────────────────────▶│
  │◀─ pong ──────────────────────────────┤
  │                                     │
  ├─ chat{text} ────────────────────────▶│
  │                                     ├─ broadcast to all clients
  │                                     ├─ emits "chat" to control hub
  │◀─ chat{from:"Vipul",text} ───────────┤ (echo to sender too)
  │                                     │
  ├─ leave (TCP close) ─────────────────▶│
  │                                     ├─ broadcast system{event:leave}
  │                                     ├─ emits "peer-leave" to control hub
  │◀─ TCP close ─────────────────────────┤
```

### UDS: control plane subscribe + action ops

```
Dashboard / citadel test              Server (or Client) process
  │                                     │
  ├─ UDS dial ──────────────────────────▶│ ~/.citadel/run/<pid>.sock
  │◀─ hello{role,name,version} ──────────┤
  │                                     │
  ├─ {"op":"subscribe","level":"full",  │
  │    "since":0} ──────────────────────▶│
  │◀─ [replay: events since seq 0] ──────┤ (ring buffer, up to 200 entries)
  │◀─ {"ev":"live"} ─────────────────────┤ (replay complete; live stream begins)
  │                                     │
  │◀─ {"ev":"peer-join","name":"Aarav"} ─┤ (live event)
  │◀─ {"ev":"chat","name":…,"text":…} ───┤
  │                                     │
  ├─ {"op":"list-peers"} ───────────────▶│
  │◀─ {"ev":"peers","peers":[…]} ─────────┤
  │                                     │
  ├─ {"op":"say","text":"hello all"} ───▶│ (server socket only)
  ├─ {"op":"kick","name":"Aarav"} ──────▶│ (server socket only)
  ├─ {"op":"send-chat","text":"hi","to":"Maya"} ─▶│ (client socket only)
  │                                     │
  ├─ {"op":"shutdown"} ─────────────────▶│
  │◀─ {"ev":"bye","reason":"shutdown"} ──┤
  │   UDS close                          │   process exits
```
