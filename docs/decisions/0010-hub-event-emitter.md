# ADR 0010 — `HubEventEmitter`, `HubLogger`, and `fanOutEmitter`

## Context

`server.Hub` had three intertwined output responsibilities, all expressed as
fields on the same struct:

1. **Event emission** — two public/private channels (`eventsCh`, `ControlEvents`)
   carrying `HubEvent` to the server TUI and control-plane bridge respectively.
   `emit()` sent to both with hardcoded non-blocking selects.
2. **Logging** — a concrete `*ActivityLog` field; every routing decision called
   methods on it directly. The type had no interface, so Hub could not be
   constructed in tests without an on-disk file (or the empty-path no-op).
3. **`ControlEvents` as a public channel field** — inconsistent with `Events()`,
   which was already a method after ADR 0002 cleanup.

None of these had injectable seams. Adding a third observer (metrics, webhook)
would require editing `Hub.emit()`. Writing tests that assert "Hub emits the
right event on kick" required reading from a real channel — possible, but not
as clean as calling into a spy.

## Decision

**Extract two interfaces and move both output channels off `Hub`.**

```go
// HubEventEmitter — output seam; one call per state change.
type HubEventEmitter interface {
    Emit(ev HubEvent)
}

// HubLogger — activity-log seam; *ActivityLog satisfies it unchanged.
type HubLogger interface {
    Join(name string)
    Leave(name string)
    Kick(name, reason string)
    Chat(name, text string)
    Direct(from, to, text string)
    Say(text string)
    Motd(text string)
}
```

`NewHub(maxClients int, motd string, log HubLogger, emitter HubEventEmitter)`
replaces the old `NewHub(maxClients int, motd string, log *ActivityLog)`.
`Hub.emit()` becomes a single `h.emitter.Emit(ev)` call.

The concrete production implementation is `fanOutEmitter`, owned by `Server`:

```go
type fanOutEmitter struct {
    tuiCh     chan HubEvent
    controlCh chan HubEvent
}
func (e *fanOutEmitter) Emit(ev HubEvent) { /* non-blocking send to both */ }
func (e *fanOutEmitter) TUIEvents() <-chan HubEvent    { return e.tuiCh }
func (e *fanOutEmitter) ControlEvents() <-chan HubEvent { return e.controlCh }
```

`Server.New()` creates the `fanOutEmitter` and passes it to `NewHub`.
`Server.EventSource()` returns `NewInProcessSource(hub, fanOut.TUIEvents())` —
the adapter used by the server TUI (see below).

**`*Hub` no longer satisfies `HubEventSource`.** The TUI interface was:
```
Events() + Kick + Say + SetMotd + Peers
```
With output channels off Hub, `*Hub` can provide the command side but not
`Events()`. An `inProcessSource` adapter (re-introduced; see Consequences)
holds `hub *Hub` for commands and `eventsCh <-chan HubEvent` from `fanOut`
for event receipt.

`bridgeHubEvents` in `server.go` reads from `s.fanOut.ControlEvents()` instead
of the former `s.hub.ControlEvents` field.

## Consequences

- **`ControlEvents` public field is gone.** Its replacement is the
  `fanOut.ControlEvents()` method — consistent with all other channel exposure
  via methods.
- **`Hub` has no output channels.** It is a pure routing engine: it receives
  commands through input channels and fires one call per event through
  `HubEventEmitter`. Adding a third observer (e.g. metrics) is a change to
  `fanOutEmitter`, not to `Hub`.
- **`HubLogger` makes Hub testable with a spy logger.** Tests inject a
  recording struct that captures `Join/Kick/…` calls; no file I/O needed.
- **`inProcessSource` is re-introduced** (deleted in ADR 0002 refactor as a
  redundant pass-through). The new version is a genuine adapter: commands go
  to `*Hub`, events come from `fanOut.TUIEvents()`. It is not a pass-through.
- **`NewTUI(hub *Hub, …)` is deleted.** Callers use `NewTUIFromSource`, which
  already existed for dashboard drill-in. The `cmd` layer calls
  `server.NewTUIFromSource(srv.EventSource(), …)`.
- **`*ActivityLog` is unchanged.** It satisfies `HubLogger` already; no
  methods were added or removed.

## Alternatives considered

- **Option A — partial emitter (bridge side only).** Keep `eventsCh` on Hub
  (preserving `HubEventSource` on `*Hub`); replace only `ControlEvents` with
  an injectable emitter. Rejected because Hub still owns one output channel,
  the inconsistency is partial, and adding a third observer still touches Hub.
  Option B costs only the `inProcessSource` adapter and a one-line change in
  `server.go`.
- **Merge `HubLogger` and `HubEventEmitter` into one `HubObserver`.** Rejected
  because logging and event emission are independent concerns with different
  consumers; a combined interface would force any test spy to implement both.

## Related

- [ADR 0007](0007-always-on-control-plane.md) — control socket always on
- [ADR 0009](0009-split-actions-provider.md) — role-specific action interfaces
- [docs/tui.md](../tui.md) — TUI architecture and HubEventSource usage
