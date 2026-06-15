---
default: tweak
---

## refactor: typed event schema, role actions, hub emitter seams, control subscriber

Internal architecture improvements across the control plane and server hub. No
changes to the wire protocol, CLI flags, or external behaviour.

- `control/event.go`: `EventKind` type + `Decode(frame)` function centralise
  all control-plane event parsing; ring-buffer entry renamed `ringEntry`
- `control/actions.go`: `ActionsProvider` replaced by `CommonActions`,
  `ServerActions`, `ClientActions`, `RoleActions` — role enforcement is now
  structural rather than runtime (`ErrNotSupported` deleted)
- `server/hub.go`: `HubEventEmitter` and `HubLogger` interfaces decouple Hub
  from its output channels and the concrete `ActivityLog`; `fanOutEmitter`
  owns both TUI and control-bridge channels
- `control/client`: `DialAndSubscribe(sockPath, level, since)` and `Subscriber`
  unify the dial → subscribe → decode sequence; callers receive
  `<-chan control.Event` instead of raw frames
- `scripts/verify.sh` + `mise run verify`: end-to-end smoke test (headless
  server + client, five control-plane scenarios)
- ADRs 0008–0011 and updated `docs/architecture.md`, `docs/control.md`,
  `docs/tui.md`, `docs/testing.md`
