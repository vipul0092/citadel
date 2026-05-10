# Citadel

Terminal LAN client/server: auto-discover a named server on your local network, connect with a unique handle, chat, and (eventually) play games — all from the terminal.

## Quickstart

```sh
# 1. Install Go via Mise (one-time per machine)
mise install

# 2. Build
go build -o citadel ./cmd/citadel

# 3a. Start the server (machine A)
./citadel server --name "Throne"

# 3b. Connect from any machine on the same network — auto-discovery, no flags needed
./citadel connect

# If auto-discovery doesn't work, use the address shown in the server TUI header:
./citadel connect --server 192.168.1.5:7777
```

### How discovery works

`citadel connect` tries three paths in parallel:
1. **Localhost probe** (300ms) — instant connection for same-machine use
2. **UDP broadcast + mDNS** — finds servers within ~2s; broadcast works on home networks even when mDNS multicast is blocked by AP isolation
3. **`--server` flag** — explicit address, always works (VPNs, different subnets, etc.)

## Usage

### Server

```
citadel server [flags]

  --name         Server display name (required)
  --port         TCP listen port (default 7777)
  --motd         Message of the day shown to joining clients
  --max-clients  Max simultaneous clients (default 16)
  --log-file     Activity log path (default ~/.citadel/<name>/activity.log)
                 Set to "" to disable logging to disk
```

### Client

```
citadel connect [flags]

  --server  Skip mDNS discovery; connect to host:port directly
  --name    Pre-fill the handle prompt
```

### In-chat slash commands

| Command | Where | Effect |
|---|---|---|
| `/who` | client | List connected peers |
| `/msg <name> <text>` | client | Direct message a peer |
| `/help` | client | Show available commands |
| `/quit` | client | Disconnect cleanly |
| `/kick <name>` | server | Kick a client |
| `/say <text>` | server | Broadcast as the server |
| `/motd <text>` | server | Update the message of the day |
| `/quit` | server | Shutdown server |

## Docs / wiki

See [`docs/README.md`](docs/README.md) for the full wiki index.

## Build from source

```sh
mise install   # installs Go + golangci-lint (one-time)
```

All common workflows are available as `mise run` tasks:

```sh
mise run build           # build the citadel binary with version timestamp
mise run build:linux     # cross-compile for Linux amd64
mise run build:windows   # cross-compile for Windows amd64
mise run test            # run all tests
mise run lint            # run golangci-lint
mise run clean           # remove build artifacts
```

Manual equivalents (if you prefer):

```sh
# Build with version timestamp embedded (recommended):
go build -ldflags "-X main.buildTime=$(date -u +%Y%m%d-%H%M%S)" -o citadel ./cmd/citadel

# Quick build during development (shows "v0.1.0-dev" in header):
go build -o citadel ./cmd/citadel

# Verify version:
./citadel version   # → v0.1.0-20260510-135000
```
