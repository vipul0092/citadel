# Protocol

## Frame format

Every message is a **length-prefixed JSON frame**:

```
┌─────────────────────┬──────────────────────────────┐
│  4 bytes (uint32 BE)│  N bytes (JSON)               │
│  payload length N   │  Envelope{...}                │
└─────────────────────┴──────────────────────────────┘
```

Max frame size: **64 KB**. Frames exceeding this are rejected and the connection is closed.

## Envelope

```go
type Envelope struct {
    Type    string          // message type (see below)
    From    string          // sender name; empty = server; never trusted from client
    To      string          // optional direct-message or kick target
    Seq     uint64          // monotonically increasing per sender
    Payload json.RawMessage // type-specific payload (see below)
}
```

## Message types

### `hello` (client → server)
Sent immediately after TCP dial. Server must receive it within 5 s.

```json
{ "name": "Vipul", "version": 1 }
```

### `welcome` (server → client)
```json
{ "server_name": "Throne", "motd": "Have fun!", "peers": ["Aarav","Maya"] }
```

### `reject` (server → client)
Server closes the connection after sending.
```json
{ "reason": "name taken" }
```

Possible reasons: `name taken`, `invalid name`, `server full`.

### `chat` (client → server, server → all clients)
`to` is empty for broadcast; non-empty for `/msg` direct messages (server routes, only target receives).
```json
{ "text": "hello!", "to": "" }
```

### `kick` (server → kicked client)
Server closes connection after sending.
```json
{ "reason": "kicked by admin" }
```

### `leave` (client → server)
Sent by client before closing connection.
```json
{}
```

### `system` (server → all clients)
Events: `join`, `leave`, `kick`, `motd`, `too_long`.
```json
{ "event": "join", "name": "Vipul", "message": "" }
```

### `ping` (client → server)
Sent every 15 s. Server drops the connection if no frame received in 60 s.
```json
{}
```

### `pong` (server → client)
```json
{}
```

### `game` (any direction)
Opaque relay. Server forwards without inspecting `kind` or `data`.
```json
{ "kind": "move", "data": { "x": 3, "y": 4 } }
```

`kind` is a game-defined discriminator. `data` is game-defined. See [features.md](features.md) for the future `kind` taxonomy.

## Name constraints
- Length: 1–24 characters
- Charset: `[A-Za-z0-9_-]`
- Case-sensitive; `Vipul` and `vipul` are different names.

## Protocol version
Current version: `1`. Clients send `version: 1` in `hello`. Server rejects unknown versions with `reject{reason:"unsupported version"}`.
