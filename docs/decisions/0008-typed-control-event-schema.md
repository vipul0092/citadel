# ADR 0008 — Typed control-plane event schema with a central `Decode` function

## Context

The control-plane wire format (JSON frames emitted by `control.Hub`) was defined implicitly:
field names (`"name"`, `"text"`, `"reason"`, etc.) appeared as magic-string literals in three separate places:

1. `control/hub.go` — inline `json.Marshal` calls that build each event's payload.
2. `server/source.go` — `frameToHubEvent`: a 55-line function doing `map[string]any` unmarshaling
   with `.(string)` type assertions to convert control-plane frames into `HubEvent` values.
3. `client/source.go` — `frameToEnvelope`: a 60-line near-duplicate doing the same for `proto.Envelope`.

Adding a new event type required editing all three files. A bug in event field parsing had to be
fixed in two places. The implicit schema made the wire format invisible: there was no single
authoritative definition of what fields each `ev` kind carried.

Both remote adapters also contained a near-identical `parsePeers` function and an identical
peer-list mutation pattern (mutex + replay-guard), further coupling the two files to a shared
but undocumented protocol shape.

## Decision

**Define the control-plane event schema as a typed Go API in `internal/control/event.go`, and
expose a single `Decode([]byte) (Event, error)` function that is the only place in the codebase
that knows the wire field names.**

Concretely:

- The internal ring-buffer entry type (`Event`) is renamed to the unexported `event`. It is used
  only within the `control` package (ring, hub, codec) and has no public callers.
- A new exported `EventKind` string type carries constants for every `ev` value the hub emits
  (`KindPeerJoin`, `KindChat`, `KindGame`, etc.).
- A new exported `PeerInfo` struct carries the peer fields that appear in `KindPeers` frames.
- A new exported `Event` struct is the decode output: a flat struct whose populated fields are
  documented per `Kind` in a Go doc comment.
- `Decode` does a single `json.Unmarshal` into a catch-all struct, then fills the appropriate
  `Event` fields based on the `ev` discriminator. Unknown kinds return a zero `Event` with `Kind`
  set and no error, so future event additions are non-breaking for existing callers.
- `server/source.go` and `client/source.go` now call `control.Decode` and switch on `ev.Kind`
  instead of doing their own `map[string]any` parsing.

Game payloads (`KindGame`) are decoded with `Name` (sender), `GameKind`, `To`, and an opaque
`Data json.RawMessage`, consistent with ADR 0003 (game payload shape is opaque to the relay).

## Consequences

- **Single source of truth for the wire format.** Adding a new event kind now requires editing
  `control/hub.go` (emit side) and `control/event.go` (decode side) only. Both remote adapters
  are updated automatically because they call `Decode`.
- **The schema is testable at the seam.** `decode_test.go` exercises the full emit → decode
  round-trip for every event kind using the real frame builders. Previously, the only test
  coverage for frame parsing was through integration tests.
- **Magic-string field names are eliminated** from `server/source.go` and `client/source.go`.
  Both files now import the `control` package and use typed constants.
- **`parsePeers` is consolidated.** The duplicate peer-parsing logic is replaced by
  `decodePeerInfos` inside `Decode`, called once for `KindPeers` frames.
- **Breaking change scope is narrow.** The only external-facing change is that `control.Event`
  (the ring buffer entry) is no longer exported. No code outside `internal/control` was using it.

## Alternatives considered

- **Keep `map[string]any` parsing but extract it into a shared helper.** This would reduce
  duplication without changing the package boundary. Rejected because it still leaves field names
  as untyped strings, gives the helper no natural home, and does not give callers a typed API.
- **Define typed structs but keep parsing in each adapter.** Rejected because it splits the
  schema definition (structs) from the implementation (parse logic), and the parse logic is still
  duplicated across two packages.

## Related

- [ADR 0005](0005-control-plane-unix-socket.md) — the UDS transport and frame format.
- [ADR 0003](0003-game-payload-shape-opaque.md) — game payloads stay opaque; `KindGame` carries
  `Data json.RawMessage` rather than decoded fields.
- [docs/control.md](../control.md) — the full op/ev catalog the control socket speaks.
