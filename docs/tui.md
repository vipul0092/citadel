# TUI

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss) + [Bubbles](https://github.com/charmbracelet/bubbles).

## Client views

### Discovery view
Live list of auto-discovered servers (UDP broadcast + mDNS). Refreshes as servers appear/disappear.

```
┌─ Citadel ─────────────────────────────────────────────────────────┐
│                                                                     │
│  Select a server  (↑↓ navigate, Enter connect, q quit)            │
│  ──────────────────────────────────────────────────────────────── │
│  ▶ Throne  192.168.1.3:7777   2 connected                         │
│    Castle  192.168.1.8:7777   0 connected                         │
│                                                                     │
│  Scanning...                                                        │
└─────────────────────────────────────────────────────────────────── ┘
```

Keys: `↑`/`↓` navigate, `Enter` connect, `q` quit.

### Name-prompt view
Text input for your handle.

```
┌─ Citadel ────────────────────────────────── server: Throne ────────┐
│                                                                     │
│  Enter your name: Vipul_                                           │
│                                                                     │
│  (1–24 chars, A-Z a-z 0-9 _ -)                                    │
└─────────────────────────────────────────────────────────────────── ┘
```

Keys: `Enter` submit, `Esc` back to discovery.

### Connected (chat) view
Split-pane. Game pane is hidden until Phase 6.

```
┌─ Citadel ─── Throne ─────────────────────────────── 3 peers ──────┐
│ [14:23:01] * Aarav joined                                          │
│ [14:24:02] Vipul: hello!                                           │
│ [14:24:10] Aarav: hey!                                             │
│ [14:25:01] → Vipul (private): hi there                            │
│                                                                     │
├─────────────────────────────────────────────────────────────────── │
│ [game: not connected]                                               │
├─────────────────────────────────────────────────────────────────── │
│ Vipul > _                                                           │
└─────────────────────────────────────────────────────────────────── ┘
```

Keys: `Enter` send, `Ctrl+C`/`/quit` disconnect, `PgUp`/`PgDn` scroll chat.

## Server admin view

```
┌─ Citadel Server: Throne ────────────────── :7777 ── 3 connected ──┐
│                                                                     │
│  CLIENTS                │  ACTIVITY LOG                           │
│  ───────────────────── │  ──────────────────────────────────────  │
│  Vipul   192.168.1.5 3m │  [14:23:01] Vipul joined               │
│  Aarav   192.168.1.6 1m │  [14:23:15] Aarav joined               │
│  Maya    192.168.1.8 30s│  [14:24:02] <Vipul> hello!             │
│                         │  [14:24:10] <Aarav> hey!               │
│                                                                     │
├─────────────────────────────────────────────────────────────────── │
│ > _                                                                 │
└─────────────────────────────────────────────────────────────────── ┘
```

Server commands: `/kick <name>`, `/say <text>`, `/motd <text>`, `/quit`.

## Split-pane design (Phase 6 game pane)

The client TUI is built with a split-pane layout from day one. The bottom game pane is hidden (zero height) until Phase 6 wires `Type:"game"` end-to-end. Revealing it requires only updating the layout proportions — no structural rewrite.

## Dashboard and drill-in

The dashboard (`citadel dashboard`) lists every running citadel process on the machine and supports `[Enter]` drill-in to a **read-only spectator** view of any server or client TUI. See [dashboard.md](dashboard.md).

Both TUIs are backed by interfaces rather than concrete types so they can be driven in-process or over a UDS control socket:

- Server admin TUI takes a `HubEventSource` — provides `Events() <-chan HubEvent` for rendering and `Kick`/`Say`/`SetMotd`/`Peers` for command dispatch. In-process: `inProcessSource` (combines `*Hub` for commands with `fanOutEmitter.TUIEvents()` for events, wired by `Server.EventSource()`). Remote drill-in: `remoteSource` (subscribes via `control/client.Subscriber`).

- Client chat TUI takes a `ConnController` — provides `Recv`/`Send`/`Name`/`Peers`/etc. In-process: `*Conn` directly satisfies the interface. Remote drill-in: `remoteConnController` (subscribes via `control/client.Subscriber`).

Remote instances are always **read-only**: command input is disabled and replaced with a spectator indicator.
