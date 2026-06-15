# ADR 0009 — Split `ActionsProvider` into role-specific interfaces

## Context

The control-plane listener (`internal/control/listener.go`) dispatches action ops from
external attachers (dashboard, test puppet, game) to the running citadel process. These
ops fall into three categories:

- **Common** — valid for both roles: `list-peers`
- **Server-only** — `kick`, `say`, `set-motd`
- **Client-only** — `send-chat`, `send-game`

The original design used a single `ActionsProvider` interface with all six methods. Both
the server and client implementations satisfied this interface, but half the methods on
each side returned `control.ErrNotSupported`:

- `serverActions.SendChat`, `serverActions.SendGame` → `ErrNotSupported`
- `connActions.KickPeer`, `connActions.SayAll`, `connActions.SetMotd` → `ErrNotSupported`

The listener handled this with a two-stage check: first `if actions == nil { notsup }`,
then `if err == ErrNotSupported { notsup }`. The interface declared a contract that half
of each implementation intentionally violated, and the call site needed two checks to
enforce what the type system could have enforced statically.

## Decision

**Replace `ActionsProvider` and `ErrNotSupported` with three focused interfaces and a
`RoleActions` bundle struct.**

```go
type CommonActions interface {
    ListPeers() []PeerInfo
}

type ServerActions interface {
    KickPeer(name, reason string) (bool, error)
    SayAll(text string) error
    SetMotd(text string) error
}

type ClientActions interface {
    SendChat(text, to string) error
    SendGame(kind, to string, data json.RawMessage) error
}

type RoleActions struct {
    Common CommonActions  // always non-nil
    Server ServerActions  // non-nil for server role; nil for client role
    Client ClientActions  // non-nil for client role; nil for server role
}
```

`control.New()` accepts `RoleActions`. Call sites pass the same concrete value for both
`Common` and their role-specific field:

```go
// server
sa := server.NewActionsProvider(hub)
control.New(..., control.RoleActions{Common: sa, Server: sa})

// client
ca := client.NewActionsProvider(conn)
control.New(..., control.RoleActions{Common: ca, Client: ca})
```

The listener dispatch uses a single nil-check per role-specific op:

```go
case "kick":
    if actions.Server == nil { notsup(op); return nil }
    ok, err := actions.Server.KickPeer(req.Name, req.Reason)
    // no ErrNotSupported path — it cannot happen
```

## Consequences

- **`ErrNotSupported` is deleted.** There is no longer a runtime path that returns it;
  role enforcement is structural.
- **Each implementation is smaller.** `serverActions` no longer carries `SendChat`/`SendGame`
  stubs; `connActions` no longer carries `KickPeer`/`SayAll`/`SetMotd` stubs.
- **The listener dispatch is honest.** Each op's guard is a single nil-check on the
  relevant role field, not a two-stage runtime probe.
- **`NewActionsProvider` returns a concrete type.** Both factory functions now return
  their unexported concrete type (`*serverActions`, `*connActions`), letting Go verify
  at the call site that the value satisfies both the `Common` and role-specific fields.
- **No change to the wire protocol.** Op names, payload shapes, and error codes are
  unchanged. The `ENOTSUP` error response still fires for role-mismatched ops — it is
  now triggered by a nil field check rather than a sentinel error value.

## Alternatives considered

- **Two interfaces (`ServerActions` + `ClientActions`) each embedding `ListPeers`.**
  Rejected because `list-peers` is genuinely a common operation; duplicating it in both
  interfaces forces an awkward branch in the listener for the one case that applies to
  both roles. A `CommonActions` interface for the shared op is cleaner.
- **Pass three separate parameters to `control.New()` instead of `RoleActions`.**
  Rejected because two of the three are always the same concrete value, and positional
  nil arguments obscure which parameter represents which role. The named struct fields
  (`Common`, `Server`, `Client`) make the intent explicit at every call site.

## Related

- [ADR 0007](0007-always-on-control-plane.md) — control socket is always on; `actions`
  was never nil in practice, making the `if actions == nil` guard in the listener dead code.
- [docs/control.md](../control.md) — the op/ev protocol the listener speaks.
