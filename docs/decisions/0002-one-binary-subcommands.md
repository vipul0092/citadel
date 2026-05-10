# ADR 0002 — One binary with subcommands

## Context

The system has two roles: server and client. Could ship as two separate binaries (`citadel-server`, `citadel-client`) or one binary with subcommands (`citadel server`, `citadel client`).

## Decision

**One binary: `citadel server …` / `citadel client …` / `citadel version`.**

## Consequences

- One file to copy to any machine; role is chosen at runtime.
- `scp citadel friend@laptop:` — done. No ambiguity about which binary to use.
- `citadel --help` is the single entry point for learning the tool.
- Easy to add future subcommands (`citadel discover` for browse-only, `citadel version`, etc.) without proliferating binaries.
- Tradeoff: slightly larger binary (both server and client code linked). Acceptable.

## Alternatives considered

- **Two binaries** — cleaner separation at the cost of two things to install. Not chosen.
