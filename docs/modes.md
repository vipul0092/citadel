# TUI mode vs headless mode

Every citadel process — server and client — can run in two modes. The mode only affects whether a terminal UI is shown; the **control plane, TCP connection, and activity log are always active** regardless of mode. See [ADR 0007](decisions/0007-always-on-control-plane.md).

## Quick comparison

| | TUI mode (default) | Headless mode (`--headless`) |
|---|---|---|
| Terminal UI | Yes — Bubble Tea full-screen | No — process runs silently |
| Control plane socket | Yes | Yes |
| TCP server / client connection | Yes | Yes |
| Activity log | Yes | Yes |
| slog output | Suppressed (TUI owns stdout) | Written to stderr / system logger |
| Suitable for CI / scripts | No | Yes |
| Suitable for background daemons | No | Yes |
| Dashboard drill-in | Yes | Yes |

## Server

### TUI mode (default)

```sh
citadel server --name Throne --port 7777
```

Launches the split-pane admin TUI: peer list on the left, activity log on the right, command input at the bottom. The TUI is the primary interface for `/kick`, `/say`, `/motd`. Quitting the TUI (`/quit` or Ctrl-C) shuts down the server.

See [tui.md](tui.md) for keybindings and layout.

### Headless mode

```sh
citadel server --name Throne --port 7777 --headless
```

The server starts, writes its sentinel file, and blocks until SIGINT/SIGTERM — no terminal is touched. Structured logs go to stderr via `slog`. Use the dashboard or `citadel test` to interact with it while it runs.

Typical uses:
- Running a server in a `tmux`/`screen` session or as a background job
- CI/integration tests that need a live server without a PTY
- Docker / systemd service

## Client

### TUI mode (default)

```sh
citadel connect                          # mDNS discovery
citadel connect --server 192.168.1.5:7777
```

Runs the full chat TUI: discovery → name prompt → chat view. The user types messages and commands directly. Quitting the TUI disconnects the client.

See [tui.md](tui.md) for keybindings.

### Headless mode

```sh
citadel connect --server 192.168.1.5:7777 --name BotPlayer --headless
```

Dials the server, completes the handshake, writes `~/.citadel/client/current.json`, then blocks until SIGINT/SIGTERM. No terminal UI is shown.

`--server` and `--name` are **required** in headless mode (there is no interactive prompt to fill them in).

The session pointer written to `~/.citadel/client/current.json` lets `citadel test` and the dashboard discover this client without scanning.

Typical uses:
- Bot players or game agents driven programmatically via `citadel test send-chat` / `send-game`
- Integration tests that need a connected peer without a PTY
- The `citadel host` wrapper, which spawns both a server and a headless client in one command

### How the client connects to the server

Discovery is **always skipped** in headless mode — `--server` is the only way to specify the target. Once the address is known, the connection sequence is the same regardless of whether the server is local or remote:

```
client.Dial(addr)          →  net.DialTimeout("tcp", addr, 10s)
conn.Handshake(name)       →  send TypeHello{name, version}
                           ←  TypeWelcome{serverName, motd, peers}  (or TypeReject → exit)
start readLoop             →  background goroutine reading TCP frames
start writeLoop            →  background goroutine draining the send queue
start pingLoop             →  TypePing every 15 s, expects TypePong
start control plane        →  opens ~/.citadel/run/<pid>.sock  (always on)
write pointer file         →  ~/.citadel/client/current.json   (headless only)
block on SIGINT/SIGTERM
```

#### Local server (same machine)

```sh
citadel server --name Lobby --port 7777 --headless &
citadel connect --server localhost:7777 --name Bot --headless &
```

The client dials `localhost:7777`. The OS routes this through the **loopback interface** (`127.0.0.1`). The wire protocol is identical to a remote connection — there is no shared-memory or Unix-socket shortcut between the two processes. Each process has its own independent UDS control socket for tooling; those sockets are not used for client↔server communication.

```
citadel connect ──── TCP loopback (127.0.0.1:7777) ────► citadel server
     │                                                         │
     └── UDS ~/.citadel/run/<client-pid>.sock        UDS ~/.citadel/run/<server-pid>.sock
         (dashboard / citadel test attach here)           (dashboard / citadel test attach here)
```

#### Remote server (different machine)

```sh
citadel connect --server 192.168.1.5:7777 --name Bot --headless &
```

Identical handshake; the TCP connection goes over the LAN instead of loopback. mDNS and UDP broadcast discovery are not attempted in headless mode, so `--server` must be the reachable IP or hostname. Cross-subnet connections require a routable address — mDNS is link-local only.

```
citadel connect ──── TCP (192.168.1.5:7777) ────► citadel server
  (machine A)                                        (machine B)
     │                                                    │
     └── UDS on machine A                    UDS on machine B
         (local tooling only)                (local tooling only)
```

Note that the UDS control sockets are always **local to their own machine** — they cannot be reached across the network.

## Interaction patterns

### Scripted test with a headless server + client

```sh
# Start a headless server in the background
citadel server --name Test --port 7778 --headless &
SERVER_PID=$!

# Connect a headless bot client
citadel connect --server localhost:7778 --name Bot --headless &
CLIENT_PID=$!

# Drive them via the test harness
citadel test send-chat --role server --text "round starting"
citadel test send-chat --text "ready"

# Tear down
kill $SERVER_PID $CLIENT_PID
```

### Dashboard + headless background server

Run the server headless so it keeps going after you detach. Use the dashboard (`citadel dashboard`) to drill in and monitor it without interrupting the session.
