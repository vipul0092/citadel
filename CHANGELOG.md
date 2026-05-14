# citadel

## 0.1.0 (2026-05-14)

### Breaking Changes

#### feat: add control plane, headless mode, and dashboard

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

### Features

- add control plane, headless mode, and dashboard

## 0.0.7 (2026-05-14)

### Updates

- Improve update command to be less verbose

## 0.0.6 (2026-05-14)

### Updates

- Add `update` command for self updating citadel

## 0.0.5 (2026-05-13)

### Updates

- Add xattr hook in release to remove quarantine property

## 0.0.4 (2026-05-13)

### Updates

- Update the action to do the actual release and brew release as well

## 0.0.3 (2026-05-13)

### Fixes

- Fix GoReleaser configuration for automated releases

## 0.0.2 (2026-05-13)

### Updates

- Add support for GoRelease automatically pushing out releases

## 0.0.1 (2026-05-13)

### Updates

- Setup the base citadel package with server/client functionality
