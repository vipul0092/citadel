# ADR 0003 — Game-message shape: opaque envelope

## Context

The wire protocol needs a game-message slot. Three plausible shapes:

A. `Type:"game"` + inner `{kind, data}` discriminator in the payload  
B. `Type:"game.move"` etc. — namespaced envelope types  
C. `Type:"game"` + `Subtype:"move"` sibling field on the envelope  

## Decision

**Option A — opaque `Type:"game"` envelope, discriminator inside the payload.**

```json
{
  "type": "game",
  "from": "Vipul",
  "payload": { "kind": "move", "data": {"x":3,"y":4} }
}
```

## Consequences

- The hub's chat/system routing logic never needs to know about game message subtypes.
- New games or new game message types only require adding `kind` values — zero envelope changes.
- A future `internal/game/` package can inspect `kind` without touching the transport layer.
- The server is a **dumb relay** for game messages in MVP: broadcast or direct-route based on `To`, never peek inside.
- Tradeoff: requires two levels of deserialization if server-side game logic ever needs to inspect game messages. Acceptable.

## Alternatives considered

- **Option B** — flat namespaced types pollute the envelope-level type space and force chat router to know game type names. Not chosen.
- **Option C** — adds a `subtype` field to every non-game envelope for no benefit. Not chosen.
