# ADR 0005 — Control plane: Unix domain socket + length-prefix JSON

## Context

We want to use Citadel as a networking sidecar for an out-of-process game (initially a TypeScript game) and to support a dashboard TUI that attaches to running citadel processes to inspect/control them. Both consumers need a stable local IPC channel into a running `citadel server` or `citadel connect`.

Four shapes were considered:

A. **Game speaks the wire protocol directly** — TS game implements length-prefix JSON, hello/welcome, ping, kick handling itself. No sidecar process; game *is* the citadel client.
B. **WebSocket on a localhost port** — citadel exposes a WS server; game and dashboard connect over `ws://127.0.0.1:<port>`.
C. **Stdio subprocess** — game spawns `citadel connect --headless` as a child and talks newline-JSON over stdin/stdout.
D. **Unix domain socket** — citadel exposes a UDS at a filesystem path; game and dashboard connect via `net.createConnection({path})`.

## Decision

**D — Unix domain socket at `~/.citadel/run/<pid>.sock`, length-prefix JSON framing (4-byte BE length + JSON), reusing the codec in `internal/proto/frame.go`.**

Each running citadel process opens its UDS listener at startup. The path is recorded in a sibling sentinel file `~/.citadel/run/<pid>.json` (see ADR 0006). Multiple attachers (game, dashboard, test puppet) may connect concurrently.

## Consequences

- **No port allocation problem.** The socket path *is* the address. No "what port did it pick", no firewall, no ephemeral exhaustion. Future multi-server-on-one-box (see [features.md](../features.md)) is unaffected.
- **Framing is reused, not reinvented.** Control frames use the same 4-byte-BE length + JSON encoding as the wire protocol. One codec, one bug surface. The envelope *contents* differ (`op`/`ev` keys vs. `Type`/`Payload`) but the framing layer is identical.
- **OS-enforced auth.** Files are created with mode `0600` — only the owning user can connect. No application-level auth required for the local-only attach scenarios v1 targets.
- **Cross-platform: macOS + Linux only.** Citadel does not target Windows; UDS is fine. If Windows support is ever needed, that becomes a separate decision (Windows ≥10 has AF_UNIX, but tooling like Node `net.createConnection({path})` works there too).
- **One fewer dependency.** No WebSocket library.
- **Stale-file cleanup.** A process that crashes leaves an orphan `.sock` file. The dashboard's sentinel scanner already prunes by pid-alive check; same loop unlinks the socket.
- **Ad-hoc debuggability.** `socat - UNIX-CONNECT:/path/to.sock` or `websocat unix:/path` works. Slightly less ergonomic than browser devtools but fine for a CLI-shaped tool.

## Alternatives considered

- **A (wire protocol direct).** Tempting because it eliminates the sidecar process — the TS game becomes a regular citadel client. Rejected because (1) the game would have to implement framing + handshake + heartbeat + reconnect + (optionally) discovery in TypeScript, duplicating logic citadel already owns; (2) the dashboard's "drill into a running client" feature requires a control-attach point that does not exist if the game *is* the client.
- **B (WebSocket).** Equivalent capability, but introduces port allocation, a third-party WS library, and asks the dashboard to manage port-collision and firewall edge cases for no benefit on a Unix-only target. Worth revisiting only if Windows becomes a requirement.
- **C (stdio).** Forces a strict 1:1 parent-child relationship: only the process that spawned citadel can talk to it. The dashboard cannot attach to a game's existing citadel-connect this way. Rejected.

## Related

- [ADR 0006](0006-session-pointer-files.md) — how the game finds its socket among many.
- [ADR 0007](0007-always-on-control-plane.md) — the control socket is on for every citadel, with or without a TUI.
- [docs/control.md](../control.md) — the on-the-wire op/ev catalog.
