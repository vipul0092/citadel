---
default: major
---

# feat: add control plane, headless mode, and dashboard

Introduce a local Unix socket control plane that every citadel process
exposes, enabling out-of-process game integration and a new dashboard TUI
for managing multiple citadel instances on one machine.

- internal/control/: always-on UDS control plane per process — ring buffer
  (200 events, seq-numbered replay), fanout hub, op/ev codec, sentinel +
  scanner, session pointer files, Go client library
- --headless flag on server/connect: run without Bubble Tea TUI; control
  plane remains active so the dashboard and game can attach
- citadel host: fork-exec wrapper that supervises server + client pair,
  writes ~/.citadel/host/current.json for game discovery
- citadel test: developer puppet (watch, send-game, send-chat, kick, drive)
- citadel dashboard: TUI listing all local citadels with launch ([h]/[c]/[s]),
  kill, restart, and full drill-in to server-admin and client-chat views
- server/client actions + source interfaces: HubEventSource/ConnController
  decoupling that backs both in-process TUI and remote dashboard drill-in
- docs: control.md, dashboard.md, ADR 0005/0006/0007, modes.md,
  integration.md, testing.md
