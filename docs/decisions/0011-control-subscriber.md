# ADR 0011 — `control/client.DialAndSubscribe` and `Subscriber`

## Context

Three separate call sites each repeated the same dial-and-subscribe sequence
against a citadel control-plane socket:

| Call site | Subscribe level | list-peers |
|-----------|----------------|------------|
| `server/source.go` `NewRemoteSource` | full | deferred to after `KindLive` |
| `client/source.go` `NewRemoteController` | full | immediate |
| `dashboard/model.go` `openSubscription` | summary | immediate |

Each one:
1. Called `ctrlclient.Dial(sockPath)`
2. Called `conn.Send({"op": "subscribe", "level": …, "since": 0})`
3. Started a goroutine ranging over `conn.Events()` (raw `[]byte` frames)
4. Called `control.Decode(frame)` on every frame (server and client sources)
   — or parsed frames manually with `json.Unmarshal` into `map[string]any`
   (dashboard)

The deletion test confirms the pattern earns a module: if `DialAndSubscribe`
were deleted, all three call sites would re-grow the same two-step handshake
and the decode step.

## Decision

**Add `DialAndSubscribe` and `Subscriber` to `internal/control/client`.**

```go
// Subscriber is a subscribed control-plane connection that delivers decoded
// control.Events.
type Subscriber struct {
    conn     *Conn
    eventsCh chan control.Event
    Hello    HelloEv
}

func DialAndSubscribe(sockPath, level string, since int) (*Subscriber, error)
func (s *Subscriber) Events() <-chan control.Event
func (s *Subscriber) Send(v any) error
func (s *Subscriber) Close()
```

`DialAndSubscribe` dials, sends the subscribe op, starts a background
`decode()` goroutine that ranges over the raw `*Conn.Events()` channel and
forwards decoded `control.Event` values to `Subscriber.eventsCh`. The channel
is closed when the underlying connection closes.

`list-peers` is NOT part of `DialAndSubscribe` — callers who need the initial
peer snapshot call `sub.Send({"op": "list-peers"})` themselves after
subscribing (or defer it as `server/source.go` does until `KindLive`).

### Impact on callers

**`server/source.go`**: `remoteSource.conn` field changes from `*Conn` to
`*Subscriber`. `translate()` ranges over `control.Event` directly; the
`control.Decode(frame)` call is removed.

**`client/source.go`**: Same field and translate changes. `list-peers` send
moves from the constructor body to after `DialAndSubscribe` returns.

**`dashboard/model.go`**: `Instance.conn` changes to `*Subscriber`.
`instanceEvent.raw []byte` becomes `instanceEvent.ev control.Event`. The
`applyEvent` function's `json.Unmarshal` + `map[string]any` string-key switch
is replaced by a typed `control.EventKind` switch:

```go
// before
evType, _ := frame["ev"].(string)
switch evType {
case "live": ...
case "peers":
    peers, _ := frame["peers"].([]any)
    instances[i].peerCount = len(peers)
case "peer-join": ...
case "peer-leave", "kick": ...
}

// after
switch ev.ev.Kind {
case control.KindLive: ...
case control.KindPeers:
    instances[i].peerCount = len(ev.ev.Peers)
case control.KindPeerJoin: ...
case control.KindPeerLeave, control.KindKick: ...
}
```

## Consequences

- **Single place for dial+subscribe boilerplate.** Adding a fourth subscriber
  (e.g. a CLI watch command) calls `DialAndSubscribe` and ranges over
  `control.Event` — no raw JSON handling.
- **Frame decoding is owned by `control/client`.** The package that speaks
  the wire format also knows how to decode it; callers work with typed events.
- **`dashboard/model.go` loses its manual JSON parsing.** `applyEvent` is now
  type-safe; the `"peers"` count uses `len(ev.ev.Peers)` instead of a
  `[]any` length, which is immune to schema drift.
- **`control/client` now imports `control`.** The dependency is `control/client`
  → `control` (child to parent). `control` does not import `control/client` in
  production code; no cycle.
- **All existing behavior is preserved.** Unknown event kinds pass through
  `Subscriber.decode()` (no error on unknown `ev`) and fall through the
  switch in callers unchanged.

## Related

- [ADR 0008](0008-typed-control-event-schema.md) — typed `Event` and central
  `Decode` that `Subscriber` now calls internally
- [docs/control.md](../control.md) — op/ev protocol the subscriber speaks
- [docs/integration.md](../integration.md) — external subscriber use cases
