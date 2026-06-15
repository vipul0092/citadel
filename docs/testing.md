# Test harness

`citadel test` is the scripted interface to the control plane. It connects to a running citadel process via its Unix domain socket and sends ops or reads events — no TUI required. Useful for integration tests, one-off automation, and debugging live sessions.

See [control.md](control.md) for the full op/event schema reference.

## End-to-end verification script

`scripts/verify.sh` is a self-contained smoke test that spins up a real server and client, runs five control-plane scenarios, and asserts the expected events are received. It cleans up all background processes on exit.

### Prerequisites

```sh
mise run build      # produces ./citadel
```

### Run

```sh
./scripts/verify.sh
# or via mise:
mise run verify
```

### What it checks

| # | Scenario | How |
|---|----------|-----|
| 1 | **peer-join** | Client connects headless; server watcher receives `ev:"peer-join"` with the client name |
| 2 | **list-peers** | `citadel test drive` sends `list-peers`; response contains `ev:"peers"` snapshot |
| 3 | **client chat** | `citadel test send-chat` from client; server watcher receives `ev:"chat"` with message text |
| 4 | **server say** | `citadel test send-chat --role server`; server watcher receives `ev:"say"` |
| 5 | **kick** | `citadel test kick`; server watcher receives `ev:"kick"` with peer name |

### Example output

```
▶  binary OK: citadel version v0.1.0
▶  starting server…
✓  server ready  addr=192.168.1.5:51234
▶  starting client…
✓  client ready
▶  scenario 1: peer-join
✓  peer-join received on server
▶  scenario 2: list-peers
✓  list-peers returned peers snapshot
▶  scenario 3: client chat
✓  chat event received on server
▶  scenario 4: server say
✓  say event received on server
▶  scenario 5: kick
✓  kick event received on server

✓  ALL CHECKS PASSED
```

### Failure output

Any assertion that does not complete within its timeout (default 8 s) prints a
failure line and exits non-zero:

```
✗  timed out waiting for '"ev":"chat"' in server.jsonl
```

The script uses a random OS-assigned port (`--port 0`) and unique process names
(`VerifyServer`, `VerifyClient`) so it is safe to run alongside a real citadel
session.

## Socket resolution

Every subcommand accepts `--sock <path>` to target a specific socket. Without it, the tool auto-resolves:

| `--role`           | Resolution order                                                                                                |
| ------------------ | --------------------------------------------------------------------------------------------------------------- |
| `client` (default) | `~/.citadel/client/current.json` → `~/.citadel/host/current.json` → scan `~/.citadel/run/*.json` for any client |
| `server`           | `~/.citadel/host/current.json` server socket → scan `~/.citadel/run/*.json` for any server                      |

If resolution fails, the command exits with an error suggesting `--sock`.

## Subcommands

### watch

Subscribe and print every event to stdout. Press Ctrl-C to stop.

```sh
citadel test watch
citadel test watch --role server
citadel test watch --level summary       # peer join/leave only; no chat
citadel test watch --sock /path/to.sock
```

Flags:

| Flag      | Default  | Description                              |
| --------- | -------- | ---------------------------------------- |
| `--sock`  | (auto)   | Control socket path                      |
| `--role`  | `client` | `client` or `server` for auto-resolution |
| `--level` | `full`   | `summary` or `full` subscription level   |

### send-chat

Send one message and exit.

```sh
# Client broadcasts to all peers
citadel test send-chat --text "round starting in 10s"

# Client sends a direct message
citadel test send-chat --text "psst" --to alice

# Server broadcasts (as the server, not a peer)
citadel test send-chat --role server --text "server says hello"
```

When `--role server`, the `say` op is used (server broadcast); `--to` is ignored.
When `--role client` (default), `send-chat` is used and `--to` is optional.

Flags:

| Flag     | Default     | Description                        |
| -------- | ----------- | ---------------------------------- |
| `--sock` | (auto)      | Control socket path                |
| `--role` | `client`    | `client` or `server`               |
| `--text` | (required)  | Message text                       |
| `--to`   | (broadcast) | Direct recipient; client role only |

### send-game

Send one game payload and exit. Always uses the client socket.

```sh
citadel test send-game --kind "start" --data '{"seed":42}'
citadel test send-game --kind "move"  --data '{"x":3,"y":4}' --to alice
```

Flags:

| Flag     | Default     | Description              |
| -------- | ----------- | ------------------------ |
| `--sock` | (auto)      | Control socket path      |
| `--kind` | (required)  | Game message kind string |
| `--data` | `{}`        | JSON payload             |
| `--to`   | (broadcast) | Direct recipient         |

### kick

Kick a peer by name via the server socket.

```sh
citadel test kick --name alice
citadel test kick --name alice --reason "idle timeout"
```

Flags:

| Flag       | Default                  | Description                                     |
| ---------- | ------------------------ | ----------------------------------------------- |
| `--sock`   | (auto)                   | Control socket path (defaults to server socket) |
| `--name`   | (required)               | Peer name to kick                               |
| `--reason` | `kicked by citadel test` | Kick reason                                     |

### drive

Read newline-delimited JSON ops from stdin and send each one. Incoming events are printed to stdout concurrently. Blank lines and lines starting with `#` are skipped.

```sh
# Interactive (type ops, Ctrl-D to finish)
citadel test drive --role server

# Pipe a script
cat script.jsonl | citadel test drive --role server

# One-liner
echo '{"op":"set-motd","text":"game starting"}' | citadel test drive --role server
```

Use `drive` when you need to send an op with no dedicated subcommand (`set-motd`, `shutdown`, `subscribe-game`, etc.), or when you want to chain multiple ops in one session.

Flags:

| Flag     | Default  | Description                              |
| -------- | -------- | ---------------------------------------- |
| `--sock` | (auto)   | Control socket path                      |
| `--role` | `client` | `client` or `server` for auto-resolution |

## Op cheat-sheet

Quick reference for ops used with `drive`. See [control.md](control.md) for full schemas.

```jsonc
// Either role
{ "op": "subscribe",      "level": "full", "since": 0 }
{ "op": "subscribe-game" }
{ "op": "list-peers" }
{ "op": "ping" }
{ "op": "shutdown" }

// Client only
{ "op": "send-chat", "text": "hello",  "to": "" }
{ "op": "send-game", "kind": "move",   "to": "", "data": {"x":1} }

// Server only
{ "op": "say",      "text": "round starts" }
{ "op": "kick",     "name": "alice", "reason": "afk" }
{ "op": "set-motd", "text": "welcome" }
```
