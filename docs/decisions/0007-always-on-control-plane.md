# ADR 0007 — Always-on control plane; `--headless` toggles TUI only

## Context

Citadel processes have two independent properties:

1. **Does it run a Bubble Tea TUI in this terminal?** (today: yes; new option: no)
2. **Does it accept control-plane attachers on its UDS socket?** (new capability from [ADR 0005](0005-control-plane-unix-socket.md))

Naïvely these look like one axis ("headless mode"), but they are orthogonal. Combinations:

|                    | TUI on           | TUI off (`--headless`)      |
| ------------------ | ---------------- | --------------------------- |
| Control socket on  | `citadel server` | `citadel server --headless` |
| Control socket off | (today)          | n/a                         |

The "control socket off + TUI off" combination is useless (a process that does nothing observable), so the real choice is whether to ship the upper-right _and_ upper-left cells, or only the upper-right.

## Decision

**Control socket is always on. `--headless` only toggles the TUI.**

Every running `citadel server` and `citadel connect`, whether or not it has a Bubble Tea TUI attached, writes its sentinel to `~/.citadel/run/<pid>.{json,sock}` and accepts control-plane attachers.

## Consequences

- **The two axes are truly independent.** "Do I want a TUI in this terminal?" and "do I want other tools to be able to attach?" are different questions; conflating them creates surprising asymmetries.
- **The dashboard discovers everything uniformly.** A `citadel server` launched from a terminal yesterday is just as drillable as one spawned by the dashboard moments ago. No two-tier "first-class headless" vs "second-class TUI'd" model.
- **The test puppet works against any citadel.** `citadel test watch` can attach to a developer's running TUI'd server during debugging without restarting it.
- **Tiny overhead.** One additional goroutine sitting in `accept(2)` on a UDS file, a fanout hub goroutine consuming `Hub.Events` (the existing TUI is already a consumer), and a ~40 KB ring buffer. Sub-noise.
- **Forward-compatible refactor.** The existing `server/tui.go` and `client/tui.go` consume `Hub.Events` / per-`Conn` events directly today. With always-on control plane, a future refactor can route the local TUI through the _same_ control hub as remote attachers — collapsing two TUI code paths into one. Not required for v1; preserved as an option.

## Alternatives considered

- **Control socket only when `--headless`.** Smaller change to existing `citadel server`. Rejected because it creates a two-tier model where the dashboard cannot see/drill-into terminal-launched citadels, which contradicts the dashboard's "unified view of every running citadel on this box" purpose. The overhead savings are negligible.

## Related

- [ADR 0005](0005-control-plane-unix-socket.md) — what the control socket is.
- [ADR 0006](0006-session-pointer-files.md) — only _session-scoped wrappers_ (host, standalone connect) write pointer files; all processes write sentinels regardless.
- [docs/control.md](../control.md) — protocol the always-on socket speaks.
