# External application integration

External applications — games, bots, automation scripts — integrate with citadel exclusively through the **UDS control socket**. They never implement the citadel wire protocol directly; instead, they attach to a running citadel process and use the op/ev JSON protocol over the socket. Citadel acts as the network layer, and the external app drives it.

See [control.md](control.md) for the full op/ev schema reference.

## Two integration scenarios

| Scenario | Server location | What the app talks to |
|----------|----------------|----------------------|
| **A — Local host** | Same machine, spawned by `citadel host` | Client socket (chat/game) + server socket (admin) |
| **B — Remote client** | Different machine | Client socket only (chat/game) |

---

## Scenario A: Local host

Use this when the app is the captain — it owns the server and is itself a participant.

### 1. Start the session

```sh
citadel host --name my-lobby --my-name Captain [--port 7777] [--motd "welcome"]
```

`citadel host` spawns two headless child processes:
- `citadel server --headless --name my-lobby --port 7777`
- `citadel connect --headless --server localhost:7777 --name Captain`

It then waits for both sentinels to appear (up to 10 s each), writes
`~/.citadel/host/current.json`, and blocks until either child exits or it
receives SIGINT/SIGTERM. If either child dies, host terminates the other
automatically.

### 2. Read the pointer file

Once `citadel host` is running, read `~/.citadel/host/current.json`:

```json
{
  "server_pid":  12345,
  "client_pid":  12346,
  "server_sock": "/Users/you/.citadel/run/12345.sock",
  "client_sock": "/Users/you/.citadel/run/12346.sock",
  "my_name":     "Captain"
}
```

Poll this file (every 100–200 ms) until it exists — `citadel host` writes it
only after both processes are confirmed ready.

### 3. Dial both sockets

Open two connections:

```
client_sock  →  send chat/game messages, receive all events
server_sock  →  admin ops only: kick, say (server broadcast), set-motd
```

The client socket is the primary channel. The server socket is optional — only
needed if the app wants to kick peers or broadcast as the server rather than as
the captain client.

### 4. Session lifecycle

```
citadel host (supervisor)
  ├── citadel server --headless  (TCP listener + control UDS)
  └── citadel connect --headless (TCP client  + control UDS)

App reads ~/.citadel/host/current.json
App dials client_sock + (optionally) server_sock
App runs the game
App sends SIGTERM to host PID  ──→  host terminates both children cleanly
```

---

## Scenario B: Remote client

Use this when the server is on another machine and the app just wants to
participate as a client.

### 1. Spawn a headless client

```sh
citadel connect --server 192.168.1.5:7777 --name BotName --headless &
CLIENT_PID=$!
```

`--server` and `--name` are both required. Discovery (mDNS / UDP broadcast) is
not attempted in headless mode.

### 2. Read the pointer file

```json
// ~/.citadel/client/current.json
{
  "client_pid":  12346,
  "client_sock": "/Users/you/.citadel/run/12346.sock",
  "server_addr": "192.168.1.5:7777",
  "server_name": "RemoteLobby",
  "my_name":     "BotName"
}
```

Poll until the file exists (citadel writes it after the TCP handshake succeeds).

### 3. Dial the client socket only

```
client_sock  →  send chat/game messages, receive all events
```

There is no server socket to dial — the server is remote and only the local
client process has a UDS socket.

---

## UDS framing

All messages on the socket use the same framing as the citadel wire protocol:

```
┌─────────────────────┬─────────────────────────┐
│  4-byte length (BE) │  N bytes of UTF-8 JSON   │
└─────────────────────┴─────────────────────────┘
```

Write the JSON, prefix it with its length as a 4-byte big-endian uint32, send
both. Read 4 bytes to get the length, then read exactly that many bytes to get
the JSON. There is no newline delimiter.

---

## Session startup sequence

After dialing either socket, follow this sequence:

```
← {"ev":"hello","role":"client","name":"Captain","version":"v0.0.7"}

→ {"op":"subscribe","level":"full","since":0}
← ... replayed ring events (chat, peer-join, peer-leave, …) ...
← {"ev":"live"}                      ← replay complete; live stream begins

→ {"op":"list-peers"}
← {"ev":"peers","peers":[{"name":"Alice","ip":"…","connected":"…"}]}

→ {"op":"subscribe-game"}            ← only if you want game payloads
```

Wait for `ev:"live"` before acting on peer counts — events before it are
replayed history. After `ev:"live"`, `peer-join` and `peer-leave` are live and
authoritative.

---

## Sending messages

### Chat (broadcast)
```json
{"op":"send-chat","text":"round starting in 10s","to":""}
```

### Chat (direct / private)
```json
{"op":"send-chat","text":"psst","to":"Alice"}
```

### Game payload (broadcast)
```json
{"op":"send-game","kind":"start","to":"","data":{"seed":42}}
```

### Game payload (to one peer)
```json
{"op":"send-game","kind":"move","to":"Alice","data":{"x":3,"y":4}}
```

`send-chat` and `send-game` are proxied through the client's existing TCP
connection. The server sees them as if the captain/bot typed them directly.

---

## Receiving events

Subscribe at `"full"` level to receive all of these. `"summary"` gives only
peer join/leave and status, no chat.

| Event | When | Key fields |
|-------|------|-----------|
| `peer-join` | A peer connected to the server | `name` |
| `peer-leave` | A peer disconnected | `name` |
| `chat` | Broadcast chat message | `name`, `text` |
| `chat-direct` | Direct message to this client | `name`, `to`, `text` |
| `say` | Server broadcast (admin `/say`) | `text` |
| `motd-changed` | MOTD was updated | `text` |
| `kick` | A peer was kicked | `name`, `reason` |
| `game` | Game payload (requires `subscribe-game`) | `from`, `kind`, `to`, `data` |
| `bye` | Citadel process shutting down | `reason` |

`ev:"bye"` means the citadel process is about to exit. Close the socket and
either restart or clean up.

---

## Admin operations (Scenario A — server socket only)

These are only available when dialed to the **server** socket. Sending them to
a client socket returns `{"ev":"error","code":"ENOTSUP"}`.

### Kick a peer
```json
{"op":"kick","name":"Alice","reason":"AFK"}
```

### Broadcast as the server
```json
{"op":"say","text":"game over"}
```

### Update MOTD
```json
{"op":"set-motd","text":"next round starts in 30s"}
```

---

## Teardown

Send SIGTERM to the `citadel host` process (Scenario A) or the headless client
process (Scenario B). The citadel process sends `ev:"bye"` to all attached
sockets, closes the UDS, and exits cleanly. Remove the pointer file if it
lingers after exit.

```sh
kill $HOST_PID    # Scenario A — terminates both server and client children
kill $CLIENT_PID  # Scenario B
```
