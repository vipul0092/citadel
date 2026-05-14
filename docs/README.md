# Citadel Wiki

| Page | Contents |
|---|---|
| [architecture.md](architecture.md) | System diagram, components, data flow |
| [protocol.md](protocol.md) | Wire format, envelope spec, all message types |
| [discovery.md](discovery.md) | localhost probe, UDP directed broadcast, mDNS, manual fallback |
| [transport.md](transport.md) | TCP+JSON framing rationale, future upgrade path |
| [tui.md](tui.md) | Bubble Tea views, keybindings, split-pane design |
| [control.md](control.md) | Control plane — UDS socket, op/ev protocol, ring buffer, replay, sentinel files |
| [dashboard.md](dashboard.md) | Dashboard TUI — multi-citadel manager, drill-in, kill/restart/spawn |
| [modes.md](modes.md) | TUI mode vs headless mode — when to use each, flags, scripting patterns |
| [integration.md](integration.md) | External application integration — UDS control socket, local host and remote client scenarios, framing, ops/events reference |
| [testing.md](testing.md) | `citadel test` harness — watch, send-chat, kick, drive, op cheat-sheet |
| [features.md](features.md) | Brainstormed backlog |
| [security.md](security.md) | Current open-LAN model, future hardening options |
| **decisions/** | Architecture Decision Records |
| [0001-language-go](decisions/0001-language-go.md) | Why Go |
| [0002-one-binary-subcommands](decisions/0002-one-binary-subcommands.md) | Single binary with subcommands |
| [0003-game-payload-shape-opaque](decisions/0003-game-payload-shape-opaque.md) | Opaque game envelope |
| [0004-discovery-mdns-plus-manual](decisions/0004-discovery-mdns-plus-manual.md) | mDNS + UDP broadcast + manual fallback |
| [0005-control-plane-unix-socket](decisions/0005-control-plane-unix-socket.md) | UDS + length-prefix JSON for local IPC |
| [0006-session-pointer-files](decisions/0006-session-pointer-files.md) | Session manifest files for game/dashboard discovery |
| [0007-always-on-control-plane](decisions/0007-always-on-control-plane.md) | Control socket always on; `--headless` toggles TUI only |
