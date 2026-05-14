# Dashboard

> **Status: design specification.** Not yet implemented as of 2026-05-14. Spec for the upcoming `citadel dashboard` subcommand. Depends on the [control plane](control.md), [ADR 0005](decisions/0005-control-plane-unix-socket.md), [ADR 0006](decisions/0006-session-pointer-files.md), [ADR 0007](decisions/0007-always-on-control-plane.md).

The dashboard is a single Bubble Tea TUI that lists every citadel process currently running on this machine, lets you launch new ones, kill or restart existing ones, and drill into any of them to use the existing server-admin or client-chat UI. It is the **first-class entry point** to a multi-citadel workflow and the canonical place from which to attach to running citadels for inspection or control.

## Top-level view

```
┌─ Citadel Dashboard ─────────────────────────────────────────  v0.0.8 ──┐
│                                                                         │
│  RUNNING                                                                │
│  ────────────────────────────────────────────────────────────────────   │
│  ▶  🏰  Throne     server   192.168.1.5:7777    3 peers   12m  ★        │
│      🔗  Vipul     client   localhost:7777      —         12m  ★        │
│      ⚔  test-srv   server   192.168.1.5:7778    0 peers   2m            │
│      🔗  debugA     client   192.168.1.5:7777    —         1m           │
│                                                                         │
│  ────────────────────────────────────────────────────────────────────   │
│   [h] Host  [c] Connect  [s] Server-only      [Enter] Open  [k] Kill    │
│   [r] Restart                                              [q] Quit     │
└──────────────────────────────────────────────────────────────────────── ┘
```

- `▶` marks the selected row.
- `★` decoration marks the entry referenced by `~/.citadel/host/current.json` or `~/.citadel/client/current.json` (the "current session").
- Trailing column is age since `started`.

## View states

```
viewDashboard      ◀───┐
       │ Enter         │ Esc
       ▼               │
viewDrilledServer ─────┤
viewDrilledClient ─────┤
       │               │
viewKillConfirm    ────┘
viewLaunchPrompt   ────┘
```

State transitions are owned by the top-level `DashboardModel`. Drilled-in views are sub-models implementing the existing `server.TUI` and `client.TUI` interfaces, with their event-source backing swapped to a remote control-plane client (see [P7 in this doc](#phasing)).

## Process discovery

On startup and on a 2-second tick:

1. Scan `~/.citadel/run/*.json`.
2. For each sentinel: verify `pid` is alive (`kill(pid, 0)` style probe). Stale → unlink both `.json` and `.sock`.
3. For each alive sentinel: ensure a `summary`-level subscription exists on its `.sock`. Open one if not.
4. Read `~/.citadel/host/current.json` and `~/.citadel/client/current.json` (if present and live) for `★` decoration.

The dashboard never mutates state of citadels it didn't spawn except via explicit user actions (`[k]`, `[r]`, drill-in commands).

## Top-level actions

### `[h]` Host a lobby

Prompts (modal in the dashboard):

```
┌─ Host a lobby ─────────────────────────────────┐
│ Lobby name (server name):  eagle-stove-7      │
│ Your name:                  Vipul              │
│ Port:                       7777 (Enter for default) │
│ Motd:                       (optional)         │
│   [Enter] Launch    [Esc] Cancel               │
└────────────────────────────────────────────────┘
```

On submit:

1. `exec.Command("citadel", "host", "--name", ..., "--my-name", ..., "--port", ...)` with `setpgid=true` (new process group, detached).
2. Wait up to 5 s for `~/.citadel/host/current.json` to appear.
3. Refresh the table; the new server and client appear with `★`.

### `[c]` Connect to a server

Prompt:

```
┌─ Connect to a server ──────────────────────────┐
│ Lobby code or address:  eagle-stove-7         │
│ Your name:              Aarav                  │
│   [Enter] Connect   [Esc] Cancel               │
└────────────────────────────────────────────────┘
```

If input matches `<name>` shape → look up via mDNS/UDP-broadcast discovery (using existing `internal/discovery/`). If it matches `host:port` → dial directly. Then `exec.Command("citadel", "connect", "--headless", ...)` detached.

### `[s]` Start server-only

Spawn `citadel server --headless` with no local client — useful for long-lived lobbies, spectator/admin scenarios, and integration testing.

## Row-level actions

### `[Enter]` Drill in

Opens the appropriate drill-in view. The drill-in view _is_ the existing TUI code — same Lipgloss styles, same keybinds, same suggestion bar — with its event source backed by a remote control-plane attacher rather than an in-process `*Hub` or `*Conn` pointer. See [phasing P7](#phasing).

Drilled-in server: `/kick`, `/say`, `/motd`, `/quit` all work and propagate through the control socket.
Drilled-in client: chat input, `/msg`, `/who`, `/quit` all work.

On entry: dashboard sends `set-level full` on the existing subscription (no new socket needed — see [docs/control.md](control.md) replay protocol).
On exit (`Esc`): dashboard sends `set-level summary` and restores the table view. The drill-in view's chat buffer is discarded; next drill-in replays from the ring buffer.

### `[k]` Kill

Sequential escalation:

1. Send `{"op":"shutdown"}` over the control socket. Wait 3 s for clean exit (sentinel disappears, socket closes).
2. If still alive: SIGTERM the pid (from sentinel). Wait 3 s.
3. If still alive: dashboard prompts "Force kill? [y/N]" — on confirm, SIGKILL.

Single confirmation modal:

```
┌─ Kill citadel? ────────────────────────────────┐
│ Throne (server, pid 12345, 3 peers connected)  │
│ Connected peers will be dropped.               │
│   [y] Yes    [N] No                            │
└────────────────────────────────────────────────┘
```

### `[r]` Restart

Read `args` from the sentinel. Run kill (graceful path only, default 3 s timeout). On exit, `exec.Command("citadel", args...)` with the same detached process-group settings. The replacement gets a new pid and therefore a new sentinel; the dashboard tracks it like any newly-spawned process.

## Process supervision policy

**The dashboard never owns the lifetime of citadel processes it spawns.** All `exec.Command` calls use `SysProcAttr{Setpgid: true}` to put the child in its own process group; the child does not die when the dashboard exits. Quitting the dashboard with `[q]` leaves every running citadel running.

This is deliberate (see [the related Q in design conversation](#)): the dashboard is a _viewer / launcher_, not a parent. Closing the dashboard window does not drop a lobby. To stop a citadel, the user explicitly `[k]`-kills it.

A future "I really want a kill-on-exit mode" could be added as a flag, but is not v1.

## Subscription bookkeeping

```
Visible row              control-plane state
─────────────             ──────────────────────────────
not drilled in   ⟶   summary subscription, ring-buffered events ignored locally
drilled in       ⟶   full subscription, ring buffer replayed on entry,
                     live stream feeds the drilled-in view's model
```

Game payloads (`ev:"game"`) are not delivered to the dashboard — it doesn't render game state, only chat/admin. The dashboard never sends `subscribe-game`.

## TUI architecture

Bubble Tea + Lipgloss + Bubbles. The dashboard is one `tea.Program` with a top-level `DashboardModel` that routes `Update`/`View` to an optional `active tea.Model` when drilled in.

```go
type DashboardModel struct {
    instances []Instance         // from `~/.citadel/run/` scan
    selected  int
    active    tea.Model          // nil = dashboard view; non-nil = drilled in
    modal     modalState         // launch prompt, kill confirm, etc.
    width, height int
    ctrl *control.Client         // multiplexed subscriptions to all visible rows
}
```

Pattern matches the existing client TUI's `viewState` enum (`viewDiscovery`/`viewNaming`/`viewChat`/`viewDisconnected`) — same router pattern, one level up.

### Drill-in backing — required refactor

Today's `internal/server/tui.go` takes `*Hub` directly; `internal/client/tui.go` takes `*Conn`. For drill-in to work, those constructors are abstracted behind interfaces:

```go
// in internal/server
type HubEventSource interface {
    Events() <-chan HubEvent
    Kick(name, reason string) bool
    Say(text string)
    SetMotd(motd string)
    Peers() []PeerEntry
}

// in internal/client
type ConnController interface {
    Recv() (*proto.Envelope, error)
    Send(msgType, to string, payload any) error
    Name() string
    ServerName() string
    Peers() []string
    Close()
}
```

Two implementations of each:

- **In-process** — wraps `*Hub` / `*Conn` (today's behavior, no functional change).
- **Remote** — backed by an `internal/control/client.Conn` plus translation between control-plane events and the existing event shapes.

The refactor lands in phase P7. Today's TUIs are unchanged until then.

## Keybinding summary

| Context       | Key     | Action                                     |
| ------------- | ------- | ------------------------------------------ |
| Dashboard     | `↑`/`↓` | Navigate rows                              |
| Dashboard     | `h`     | Host a lobby (modal)                       |
| Dashboard     | `c`     | Connect to server (modal)                  |
| Dashboard     | `s`     | Start server-only (modal)                  |
| Dashboard     | `Enter` | Drill in to selected row                   |
| Dashboard     | `k`     | Kill selected row (confirm)                |
| Dashboard     | `r`     | Restart selected row                       |
| Dashboard     | `q`     | Quit dashboard (does not kill any citadel) |
| Drill-in view | `Esc`   | Back to dashboard                          |
| Drill-in view | (rest)  | Same as standalone TUI                     |
| Modal         | `Esc`   | Cancel                                     |
| Modal         | `Enter` | Confirm                                    |

## Phasing

The dashboard is the last major piece in the headless-mode rollout. Order of work:

- **P1** — Control plane infrastructure (`internal/control/`), always-on UDS socket, sentinel files. No user-visible change.
- **P2** — `--headless` flag on `citadel server`/`connect`. Standalone `connect --headless` writes `~/.citadel/client/current.json`.
- **P3** — `citadel host` wrapper. Writes `~/.citadel/host/current.json`. Supervises lifetime of its two children.
- **P4** — `citadel test` subcommand (puppet). Reuses control-plane client library.
- **P5** — Integration tests spawning real subprocesses, driving via the Go control client.
- **P6** — Dashboard skeleton: scan, table, launch + kill + restart. **No drill-in yet** — only top-level functionality.
- **P7** — Refactor `server/tui.go` + `client/tui.go` behind `HubEventSource` / `ConnController` interfaces. Drill-in lands now, using the remote backing.
- **P8** — Polish, docs, `--port 0` support, [features.md](features.md) cleanup.

## Multi-server (future)

The architecture is already multi-server-capable; only the v1 defaults are single-server-oriented:

- The sentinel directory naturally lists N entries.
- `--port 0` support lands in P8 so the kernel can hand out ports; the dashboard can begin passing `--port 0` to `[s]` and `[h]` once we want N concurrent servers per box.
- `~/.citadel/host/current.json` becomes the "current default session" while richer multi-session UX is layered on as a follow-up.

Not in v1 scope: explicit per-session pointer files (e.g. `<session_id>.json`), in-dashboard session switching, cross-process session naming. These are unblocked by the architecture but deliberately deferred.
