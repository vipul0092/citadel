# ADR 0001 — Language: Go

## Context

Need a language for a terminal LAN client/server tool that:
- Produces a single distributable binary (easy to copy to another machine)
- Has excellent networking primitives (TCP, mDNS)
- Has a high-quality terminal UI library
- Compiles cross-platform (macOS, Linux, Windows)
- The developer is primarily a TypeScript/Java engineer but open to learning

## Decision

**Go.**

## Consequences

- `go build` produces a single static binary — no runtime to install on target machines.
- Goroutines map naturally to the per-client actor model (one goroutine reads, one writes, hub owns shared state).
- Bubble Tea + Lipgloss is best-in-class for terminal UIs in any language.
- `grandcat/zeroconf` provides solid mDNS without needing system libraries.
- Cross-compile with `GOOS`/`GOARCH` env vars, no extra tooling.
- Tradeoff: new language. Manageable given Go's small surface area.

## Alternatives considered

- **TypeScript + Node.js** — fastest iteration given existing expertise; distribution requires bundling a Node runtime (pkg/bun). `ink` TUI is good but less powerful than Bubble Tea. Not chosen.
- **Rust** — strongest type-safety, ratatui TUI is excellent. Longer compile cycles, steeper ramp. Not chosen.
- **Python** — easiest to prototype; PyInstaller binaries are heavy; Textual TUI is new. Not chosen.
