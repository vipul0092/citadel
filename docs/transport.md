# Transport

## Current: TCP + length-prefixed JSON

Every message is a 4-byte big-endian length header followed by a JSON-encoded `Envelope`. Max frame: 64 KB.

**Why plain TCP?**
- Simple to implement and debug (readable with `tcpdump`)
- Ordered, reliable delivery — needed for chat and command flow
- No external protocol overhead (no WebSocket handshake, no HTTP layer)
- Cross-platform without extra deps

**Why JSON?**
- Human-readable, easy to debug with `tail -f activity.log | jq .`
- No schema pre-registration needed during rapid iteration
- Negligible performance overhead for the message volume expected (chat + game commands)

## Future: msgpack or protobuf

If game payloads grow heavy (high-frequency state updates, binary assets), consider:
- **msgpack** — drop-in binary JSON; lower wire size, faster parse; add `github.com/vmihaiela/msgpack` with a `Content-Type` handshake at connect time
- **protobuf** — stronger typed contracts; worth adding when the game protocol stabilises

The framing layer (`internal/proto/frame.go`) is encoding-agnostic — only the envelope deserialiser needs changing. A content-type negotiation in the `hello` payload would allow backwards-compatible rollout.

## Max frame size: 64 KB

Prevents a malicious or buggy client from OOMing the server by sending a giant frame. If a game needs larger payloads (map data, textures), consider:
- Chunking at the application layer
- A separate file-transfer channel
- Raising the cap via a `--max-frame` server flag (add to backlog)
