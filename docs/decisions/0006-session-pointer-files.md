# ADR 0006 — Session pointer files

## Context

[ADR 0005](0005-control-plane-unix-socket.md) puts every citadel process's control socket at `~/.citadel/run/<pid>.sock`. With multiple citadels potentially running on one machine — a server, the host's own client, a leftover debug instance, a test puppet's spawn — the TypeScript game (and other attachers) need a deterministic way to find **its** socket, not just **a** socket.

Three approaches:

A. **Game scans `~/.citadel/run/` itself.** Filter by `role`, pick the most-recently-started, or match `parent_pid` heuristically.
B. **Well-known pointer file per session type.** A session-scoped wrapper writes a single file at a fixed path containing the relevant socket paths.
C. **Explicit env vars or wrapper.** `citadel host --launch ./game.js` fork-execs the game with `CITADEL_*_SOCK=...` set; or `eval $(citadel host --envrc)` exports them.

## Decision

**B — well-known session pointer files.**

```
~/.citadel/host/current.json     # written by `citadel host`
~/.citadel/client/current.json   # written by standalone `citadel connect --headless`
```

`~/.citadel/host/current.json` schema:

```json
{
  "server_sock": "/Users/vgaur/.citadel/run/12345.sock",
  "server_pid":  12345,
  "server_name": "Throne",
  "client_sock": "/Users/vgaur/.citadel/run/12346.sock",
  "client_pid":  12346,
  "my_name":     "Vipul",
  "started_at":  "2026-05-14T10:23:01Z"
}
```

`~/.citadel/client/current.json` schema (subset — no server side):

```json
{
  "client_sock": "/Users/vgaur/.citadel/run/12350.sock",
  "client_pid":  12350,
  "server_addr": "192.168.1.5:7777",
  "server_name": "Throne",
  "my_name":     "Aarav",
  "started_at":  "2026-05-14T10:24:11Z"
}
```

Written on session start by the responsible wrapper (`citadel host` or `citadel connect --headless`). Removed on clean shutdown.

## Consequences

- **Trivial for the TS game.** One `fs.readFileSync` + `JSON.parse` and the game has both socket paths it needs.
- **Captain identity is implicit.** A game that finds a `server_sock` in `host/current.json` is on the host's box and can attach for kick/admin ops. A game that finds only `client/current.json` is a guest. No new protocol surface.
- **One session at a time per user.** The `current.json` filename bakes in this constraint. For v1 this is a feature, not a limitation — a single user does not host two simultaneous lobbies. If multi-session is needed later, rename to `<session_id>.json` with `current.json` as a symlink. Forward-compatible.
- **Stale-file cleanup is uniform.** Any reader detecting a stale pointer (pid no longer alive) unlinks it. Same loop used for `~/.citadel/run/*.json` sentinels.
- **Dashboard signal.** The dashboard can decorate matching rows in its table as "current host session" / "current join session" — useful when there are several test/debug citadels on disk.

## Alternatives considered

- **A (scan run/).** Self-contained but requires heuristics ("most recent", "parent_pid match") that are wrong in real scenarios — running a fresh test process while a host session is live silently steals it. Rejected.
- **C (env vars / wrapper).** Couples citadel to game-launching, or forces the user to chain shell commands. Breaks the common case where the game is launched from an IDE, debugger, or `npm run`. The point of headless mode is to *decouple* citadel and the game. Rejected.

## Related

- [ADR 0005](0005-control-plane-unix-socket.md) — what the sockets these files point to actually are.
- [ADR 0007](0007-always-on-control-plane.md) — every citadel has a control socket; only session-scoped wrappers write pointer files.
- [docs/control.md](../control.md) — discovery section.
