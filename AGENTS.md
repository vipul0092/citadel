# Citadel — CLAUDE.md

## Doc-sync rules

- **Single source of truth**: when an architectural change is requested, update the relevant `docs/` page **before or alongside** the code change — never let docs drift behind.
- **Backlog hygiene**: when a feature lands in code, remove it from `docs/features.md` and add a section to its owning doc (e.g. ping/pong moves from features into `docs/protocol.md`).
- **ADRs**: any decision that closes off alternatives (transport format swap, auth model change, framing change) → new file `docs/decisions/NNNN-<slug>.md` with Context / Decision / Consequences.
- **Index discipline**: every new page added to `docs/` must get a line in `docs/README.md`.
- **"Update the wiki" command**: when the user says "update the wiki", scan the recent conversation for decisions, write/update the relevant pages in `docs/`, and summarise what changed.
- **Match scope**: don't preemptively document hypothetical features — only document what is decided or implemented.

## Go code conventions

- **Format**: `gofmt` mandatory; `golangci-lint` (with `errcheck`, `govet`, `staticcheck`, `revive`) on all changed files.
- **Errors**: wrap with `fmt.Errorf("...: %w", err)`, lowercase first letter, no trailing punctuation.
- **Logging**: `log/slog` (structured) everywhere in `internal/` — never `log` or `fmt.Println`.
- **Tests**: table-driven where it pays, colocated `_test.go` files, `t.Helper()` in shared test helpers.
- **Naming**: short receiver names (`s *Server`, not `srv`); export only when crossing package boundaries.
- **Concurrency**: prefer channels over mutexes for owned state; use mutexes only for tightly-scoped guards (e.g., a sync.Once).

## Project overview (for quick orientation)

```
cmd/citadel/        single binary — server|client|version subcommands
internal/proto/     wire protocol: frames, envelopes, message payloads
internal/discovery/ mDNS advertise + UDP directed broadcast (server); browse + broadcast listener (client)
internal/server/    hub, per-connection actors, activity log, admin TUI
internal/client/    conn, slash commands, chat TUI
docs/               wiki — architecture, protocol, features backlog, decisions
```

Module: `github.com/vipul0092/citadel`  
Toolchain: Go 1.25.0 managed via Mise (`mise install` before first build).

## Build

All workflows go through Mise tasks defined in `mise.toml`:

```
mise run build   # build the citadel binary with version timestamp
mise run test    # run all tests
mise run lint    # run golangci-lint
mise run clean   # remove build artifacts
```

**Quality gate**: every change must pass `mise run build`, `mise run test`, and `mise run lint` before it is considered complete. Run all three after any code change.
