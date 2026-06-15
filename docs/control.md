# Control plane

Related ADRs: [0005](decisions/0005-control-plane-unix-socket.md), [0006](decisions/0006-session-pointer-files.md), [0007](decisions/0007-always-on-control-plane.md).

The control plane is the local IPC channel between a running citadel process and any number of attachers — typically the dashboard TUI, the `citadel test` puppet, and (most importantly) an out-of-process game written in another language. It is **separate from** the wire protocol on TCP that citadel servers and clients use to talk to each other; it is the API by which the rest of the local system talks to a single citadel.

## How it works

### Two sockets, two purposes

Every citadel process opens **two completely separate sockets** with no overlap:

| Socket | Protocol | Who connects | Purpose |
|--------|----------|-------------|---------|
| **TCP `host:port`** | Citadel wire protocol | Remote peers on any machine | Chat, game messages, join/leave |
| **UDS `~/.citadel/run/<pid>.sock`** | Control plane (this doc) | Local tools on the same machine only | Dashboard, `citadel test`, game process |

The TCP socket is the game network. The UDS socket is the local admin/IPC interface. Nothing you send to the UDS touches the TCP path, and vice versa.

### What is a Unix Domain Socket?

A Unix Domain Socket (UDS) works exactly like a TCP socket — you dial it, read from it, write to it — except the address is a **filesystem path** instead of `host:port`. The kernel handles all buffering; there is no network stack involved and no routing. Because the address is a path, it inherits filesystem permissions: citadel creates the socket as mode `0600` so only the process owner can connect. Nothing on the network can reach it.

### Lifecycle

When a citadel process starts (`control.New` in `plane.go`):

1. `openListener` calls `net.Listen("unix", "~/.citadel/run/<pid>.sock")` — creates the socket file on disk and starts an accept loop in a goroutine.
2. `WriteSentinel` writes `~/.citadel/run/<pid>.json` — a discovery manifest recording the socket path alongside role, name, TCP address, and original `os.Args` (used by the dashboard's restart feature).
3. On shutdown, the stop function closes the listener and removes the socket file; `Sentinel.Unlink()` removes the JSON.

Both files are `0600`. Stale files from a crashed process are cleaned up by any reader that notices the recorded pid is no longer alive.

### How an attacher connects

1. **Discover**: scan `~/.citadel/run/*.json` (`control.ScanSentinels`) and read the `control` field from each sentinel to get the socket path. The dashboard does this every 2 seconds.
2. **Dial**: `net.Dial("unix", sockPath)` — standard Go; no special library needed.
3. **Hello**: citadel immediately sends `{"ev":"hello","role":"...","name":"...","version":"..."}`. The attacher reads this to learn what kind of process it has connected to.
4. **Subscribe**: attacher sends `{"op":"subscribe","level":"full","since":0}`. Citadel replays buffered events from the ring, then sends `{"ev":"live"}`, then streams live events.
5. **Act**: attacher sends action ops (`kick`, `say`, `send-chat`, etc.). Citadel routes them through role-specific action interfaces (`CommonActions`, `ServerActions`, or `ClientActions` from `control/actions.go`) into the real server hub or client connection.

Multiple attachers can be connected simultaneously. Each gets its own independent subscription state managed by `sub.go`; the hub fan-outs to all of them.

### Message relay — how send-chat and send-game actually work

A game process or bot that attaches to a **client** socket and sends `send-chat` or `send-game` is not talking to the server directly. It is asking the citadel client process to speak on its behalf over the client's existing TCP connection. The UDS is a proxy, not a bypass.

**Outbound path (attacher → server):**

```
Game / bot
  │  {"op":"send-chat","text":"hello","to":""}
  ▼
UDS control socket  (serveAttacher in control/listener.go)
  │  handleOp → actions.SendChat(text, to)
  ▼
connActions.SendChat  (client/actions.go)
  │  conn.Send(TypeChat, to, ChatPayload{...})
  ▼
conn.sendCh  (buffered channel, cap 64)
  │
  ▼
client writeLoop  →  TCP frame  →  server Hub  →  broadcast to all peers
```

**Inbound path (server → attacher):**

```
server Hub  →  TCP frame  →  client readLoop
                                    │
                     ┌──────────────┴──────────────┐
                     ▼                             ▼ (lossy non-blocking tap)
               conn.recvCh                   conn.ctrlCh  (cap 32, dropped if full)
               (Chat TUI /                         │
                headless Recv())              bridgeLoop
                                                   │
                                        ctrl.Hub().ForwardEnvelope(env)
                                                   │
                                        control.Hub.Emit("chat", data)
                                                   │
                                        ring buffer + fan-out to subscribers
                                                   │
                                        ev:"chat" on UDS  →  Game / bot
```

Three things to keep in mind:

- **The inbound tap is lossy.** `ctrlCh` is filled with a non-blocking send; if the bridge goroutine is behind, the frame is silently dropped for the control plane. The TUI/`recvCh` always gets the frame regardless.
- **Game payloads bypass the ring.** `TypeGame` frames are forwarded via `EmitGame`, which delivers directly to `subscribe-game` subscribers without writing to the ring buffer. There is no replay for game events on reconnect.
- **Server-side attachers cannot send chat.** `send-chat` and `send-game` are client-only ops. Sending them to a server socket returns `ENOTSUP`. Use `say` to broadcast as the server.

### Shared framing

The UDS control plane reuses the same framing as the TCP wire protocol: a **4-byte big-endian length prefix** followed by N bytes of JSON (`internal/proto/frame.go`). This is why `proto.ReadFrame`/`proto.WriteFrame` appear in both `internal/server/hub.go` and `internal/control/listener.go`. The JSON schema is different (op/ev model here vs Envelope on TCP), but the wire encoding is identical.

## Transport

- Unix domain socket, mode `0600`, path `~/.citadel/run/<pid>.sock`.
- Framing: 4-byte big-endian length prefix followed by N bytes of JSON. **Reuses `internal/proto/frame.go`** — the same encoder/decoder as the wire protocol.
- Max frame: 64 KB (matches wire-protocol limit).
- Bidirectional: attachers send `op` messages; citadel sends `ev` messages. Both flow over the same socket.
- Multi-attach: a citadel process accepts many concurrent attachers. Each gets its own subscription state and fan-out.

## Discovery

Two filesystem conventions cooperate:

### Sentinel file — written by every citadel process

`~/.citadel/run/<pid>.json`, written on startup, removed on clean shutdown:

```json
{
  "pid":       12345,
  "role":      "server",
  "name":      "Throne",
  "addr":      "192.168.1.5:7777",
  "control":   "/Users/vgaur/.citadel/run/12345.sock",
  "args":      ["server", "--name", "Throne", "--port", "7777"],
  "started":   "2026-05-14T10:23:01Z"
}
```

For clients, `role` is `"client"`, `addr` is the *remote* server, and `name` is the client's own handle.

The `args` field captures the original argv (minus the program path) so the dashboard's `[r] Restart` can spawn a replacement with the same configuration.

### Session pointer files — written by session-scoped wrappers only

`~/.citadel/host/current.json` (by `citadel host`) and `~/.citadel/client/current.json` (by standalone `citadel connect --headless`). Schemas in [ADR 0006](decisions/0006-session-pointer-files.md).

These exist to give the TS game a single deterministic file to read — it does not need to scan `run/`.

### Cleanup

Any reader that finds a sentinel whose `pid` is no longer alive should unlink both the `.json` and the `.sock`. Same for stale pointer files. The dashboard does this on every scan; standalone tools may do it lazily.

## Op/ev model

Attachers send **ops**. Citadel sends **events**. Every message is a JSON object with a single discriminator at the top:

```json
{ "op": "subscribe", "level": "full", "since": 0 }       // attacher → citadel
{ "ev": "chat", "seq": 42, "at": "...", "name": "Vipul", "text": "hi" }   // citadel → attacher
```

The discriminator (`op` or `ev`) is the first field and identifies the message shape.

## Ops (attacher → citadel)

| op            | applies to            | description |
|---------------|-----------------------|-------------|
| `subscribe`   | any                   | Begin receiving events. Sets level and optional `since` for ring-buffer replay. |
| `set-level`   | any                   | Change subscription level on an existing connection. |
| `subscribe-game` | any                | Begin receiving live `Type:"game"` payloads. No replay; live only. |
| `unsubscribe-game` | any              | Stop receiving game payloads. |
| `send-chat`   | client                | Send a chat message (broadcast or direct via `to`). |
| `send-game`   | client                | Send a `Type:"game"` payload with arbitrary `kind` and `data`. |
| `list-peers`  | server, client        | Request a snapshot of currently-connected peers. |
| `kick`        | server                | Kick a peer by name with a reason. |
| `say`         | server                | Server broadcast message (admin /say). |
| `set-motd`    | server                | Update the motd. |
| `shutdown`    | any                   | Graceful shutdown (broadcast leave, close listener, exit). |
| `ping`        | any                   | Control-plane keepalive (distinct from wire-level ping). |

### Op schemas

```json
// Begin a subscription. Replays the ring buffer from `since` if provided.
{ "op": "subscribe",
  "level":   "summary" | "full",
  "since":   <uint64, default 0>            // last seq the attacher already has
}

// Change level on this connection. Triggers a replay if upgrading.
{ "op": "set-level",
  "level":   "summary" | "full",
  "since":   <uint64, default 0>
}

// Opt in to live game payload stream. No replay.
{ "op": "subscribe-game" }
{ "op": "unsubscribe-game" }

// Client only.
{ "op": "send-chat", "text": "hello", "to": "" }            // empty to = broadcast
{ "op": "send-game", "kind": "move", "to": "", "data": {...} }

// Either role; returns ev:"peers".
{ "op": "list-peers" }

// Server only.
{ "op": "kick",     "name": "Aarav", "reason": "afk" }
{ "op": "say",      "text": "round starts in 30s" }
{ "op": "set-motd", "text": "have fun!" }

// Either role.
{ "op": "shutdown" }
{ "op": "ping" }
```

## Events (citadel → attacher)

| ev               | direction          | description |
|------------------|--------------------|-------------|
| `hello`          | sent on connect    | First frame after attach. Echoes role, name, version. |
| `status`         | summary + full     | Periodic snapshot. Peer count, motd, uptime. |
| `peers`          | response to `list-peers` | Current peer list. |
| `peer-join`      | summary + full     | A peer joined. |
| `peer-leave`     | summary + full     | A peer left. |
| `chat`           | full               | Broadcast chat. |
| `chat-direct`    | full               | Direct (`/msg`) chat. |
| `say`            | full               | Server `/say` broadcast. |
| `motd-changed`   | full               | Motd was updated. |
| `kick`           | full               | A peer was kicked. (Client view: includes self-kick.) |
| `system`         | full               | Other wire-protocol system events (e.g. `too_long`). |
| `game`           | subscribe-game     | Live game payload. Never buffered in the ring. |
| `live`           | replay → live      | Marker emitted once between replay and live events on (re)subscribe. |
| `gap`            | replay edge        | Sent when `since` is older than the ring's oldest entry — attacher should treat the next snapshot as authoritative. |
| `error`          | any                | Protocol or operational error. |
| `pong`           | response to `ping` | Control-plane keepalive ack. |
| `bye`            | sent before close  | Citadel is shutting down. |

### Event schemas

Every event carries `seq` (monotonic per process; never reset until process restart) and `at` (RFC3339 timestamp).

```json
{ "ev": "hello", "role": "server" | "client", "name": "<self-name>", "version": "v0.0.7" }

{ "ev": "status",     "seq": 42, "at": "...", "peers": 3, "motd": "...", "uptime_sec": 1234 }

{ "ev": "peers",      "seq": 43, "at": "...", "peers": [{"name":"Vipul","ip":"...","connected":"..."}] }
{ "ev": "peer-join",  "seq": 18, "at": "...", "name": "Aarav" }
{ "ev": "peer-leave", "seq": 19, "at": "...", "name": "Aarav" }

{ "ev": "chat",        "seq": 44, "at": "...", "name": "Vipul", "text": "hi" }
{ "ev": "chat-direct", "seq": 45, "at": "...", "name": "Vipul", "to": "Aarav", "text": "psst" }
{ "ev": "say",         "seq": 46, "at": "...", "text": "round starts" }
{ "ev": "motd-changed","seq": 47, "at": "...", "text": "new motd" }

{ "ev": "kick",        "seq": 48, "at": "...", "name": "Aarav", "reason": "afk" }
{ "ev": "system",      "seq": 49, "at": "...", "event": "too_long", "name": "Vipul" }

{ "ev": "game",        "from": "Aarav", "kind": "move", "to": "", "data": {...} }   // no seq

{ "ev": "live" }
{ "ev": "gap",   "missing_from": 18, "missing_to": 41 }
{ "ev": "error", "code": "EINVAL", "msg": "unknown op" }
{ "ev": "pong" }
{ "ev": "bye",   "reason": "shutdown" }
```

Note: `ev:"game"` carries no `seq` — game payloads are live-stream only and excluded from the ring buffer. See ADR 0003 ("server is a dumb relay for game messages") and the *Ring buffer* section below.

## Subscription levels

| Level     | Includes |
|-----------|----------|
| `summary` | `status`, `peers`, `peer-join`, `peer-leave` |
| `full`    | summary set + `chat`, `chat-direct`, `say`, `motd-changed`, `kick`, `system` |

Orthogonal opt-ins on the same connection:

- `subscribe-game` → live `game` events. Stops on `unsubscribe-game` or on `set-level`.

The dashboard subscribes every visible row at `summary` (cheap) and upgrades to `full` only on drill-in. The TS game typically wants `subscribe-game` + (optionally) `summary` to track peer changes for its lobby UI.

## Ring buffer (replay)

Each citadel process maintains a single in-memory ring buffer of recent **control-plane** events (everything except `game` and `ping`/`pong`/heartbeat noise).

- Capacity: **200 entries**, fixed in v1.
- Each entry is an `Event` with monotonic `seq` (per-process counter that never resets within a pid's lifetime).
- Game payloads (`Type:"game"`) and heartbeats are **never** buffered. At 30–60 Hz × N players a game-payload buffer would overflow in under a second, and replay is meaningless — game state is reconstructed by the game's own logic, not from citadel history. This matches [ADR 0003](decisions/0003-game-payload-shape-opaque.md): citadel relays game payloads opaquely.
- Persistence: none. Ring is in-memory, resets per pid. Long-term records remain in `~/.citadel/<name>/activity.log`.

### Replay protocol

When an attacher subscribes (or upgrades level), citadel replays buffered events with `seq > since` matching the target level, then emits `{"ev":"live"}`, then resumes streaming. The lock that protects the ring also blocks `Emit` during replay, so no live event arrives before the marker.

If `since` is older than the oldest entry currently in the ring, citadel emits `{"ev":"gap", "missing_from": X, "missing_to": Y}` before the replay. The attacher should treat the next `status` event as authoritative state and reconcile.

## Slow-attacher policy

Each attacher has a 64-message outbound queue (mirroring `internal/server/client.go:43-52`). If the queue is full when `Emit` fires, the attacher is dropped: its channel is closed and removed from the subscriber set. The producer never blocks on a slow consumer.

The dashboard handles this by treating connection close as "process unreachable" — it removes the row temporarily and re-attaches on the next scan tick.

## Reconnect

Connections may close due to:
- Citadel process shutdown (orderly: `bye` event then close)
- Attacher process exit (close)
- Slow-attacher kick (close, no `bye`)
- Filesystem socket file unlinked (close)

The Go control-plane client library (used by the dashboard, the puppet, and integration tests) implements a standard reconnect loop: exponential backoff capped at 5 s, surfacing connection state to the consumer. On successful reattach, the consumer re-issues `subscribe` with `since: <last_seen_seq>` to recover any events that landed during the gap.

The TS game library should follow the same pattern. See `internal/control/client/` for the reference Go implementation.

## Error codes

| Code            | Meaning |
|-----------------|---------|
| `EINVAL`        | Malformed op or unknown discriminator. |
| `ENOTSUP`       | Op valid but not applicable to this role (e.g. `kick` to a client socket). |
| `ENOENT`        | Target not found (e.g. `kick name=Aarav` when Aarav is gone). |
| `EBUSY`         | Op cannot proceed in current state (rare). |
| `EDROPPED`      | Attacher was dropped for slow consumption (delivered just before close where possible). |
| `EINTERNAL`     | Unexpected server-side error; details in `msg`. |

## Wire example — TS game (host side) joining the lobby

```
1. host runs:   citadel host --name eagle-stove-7 --my-name Vipul
                → writes ~/.citadel/host/current.json
                → server pid 12345, client pid 12346

2. game reads:  current.json → server_sock, client_sock

3. game opens   the client socket (length-prefix JSON framing)
   ←  { "ev":"hello", "role":"client", "name":"Vipul", "version":"v0.0.7" }
   →  { "op":"subscribe",      "level":"summary", "since": 0 }
   →  { "op":"subscribe-game" }
   ←  { "ev":"status", "seq":1, "peers":0, "uptime_sec":0, "at":"..." }
   ←  { "ev":"live"  }

4. as guests join:
   ←  { "ev":"peer-join", "seq":2, "name":"Aarav", "at":"..." }
   ←  { "ev":"peer-join", "seq":3, "name":"Maya",  "at":"..." }

5. captain UI shows the lobby; user presses "Start"; game sends:
   →  { "op":"send-game", "kind":"start", "to":"", "data":{"seed":42} }

6. citadel relays to all peers; their games receive:
   ←  { "ev":"game", "from":"Vipul", "kind":"start", "to":"", "data":{"seed":42} }
```

For captain-only operations the game ALSO attaches to `server_sock`:

```
7. game opens server socket
   ←  { "ev":"hello", "role":"server", "name":"eagle-stove-7", "version":"v0.0.7" }
   →  { "op":"subscribe", "level":"summary", "since": 0 }
   (later, captain decides to remove a player)
   →  { "op":"kick", "name":"Maya", "reason":"AFK" }
   ←  { "ev":"kick", "seq":17, "name":"Maya", "reason":"AFK", "at":"..." }
   (kick also propagates over the wire protocol — Maya's client emits ev:"kick" on her socket)
```

## Reference implementation

- `internal/control/` — server-side fanout hub + ring buffer + UDS listener. `event.go` owns `EventKind` constants, `Event` struct, and the central `Decode` function.
- `internal/control/client/` — Go client library. `Conn` provides raw dial+send+recv. `Subscriber` + `DialAndSubscribe(sockPath, level, since)` handle the full dial → subscribe → decode pipeline, delivering `<-chan control.Event` to callers. Consumed by dashboard, `citadel test`, and integration tests.
- `cmd/citadel/test_*.go` — `citadel test` subcommand wiring.

See [docs/dashboard.md](dashboard.md) for the consumer UX layered on top.
