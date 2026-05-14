# Citadel

Terminal LAN client/server: auto-discover a named server on your local network, connect with a unique handle, chat, and (eventually) play games — all from the terminal.

## Prerequisites

Install [Mise](https://mise.jdx.dev/getting-started.html) (tool version manager):

```sh
curl https://mise.run | sh   # macOS / Linux
```

## Quickstart

```sh
# 1. Install toolchain (one-time per machine)
mise install

# 2. Build
mise run build

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
2. **UDP broadcast + mDNS** — finds servers within ~10s; broadcast works on home networks even when mDNS multicast is blocked by AP isolation
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

### Update

```
citadel update
```

Updates citadel to the latest version via Homebrew. Requires Homebrew to be installed.

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
mise install   # installs Go + golangci-lint + knope (one-time)
```

All common workflows are available as `mise run` tasks:

```sh
mise run build           # build the citadel binary
mise run test            # run all tests
mise run lint            # run golangci-lint
mise run clean           # remove build artifacts
```

## Install via Homebrew

```sh
brew tap vipul0092/citadel https://github.com/vipul0092/citadel
brew install citadel
```

To update to the latest version:

```sh
citadel update
```

## Releasing

Versions are managed by [Knope](https://knope.tech/) using changeset files. Binaries are published by [GoReleaser](https://goreleaser.com/) via GitHub Actions.

```sh
# 1. Document a change (interactive — pick major/minor/patch/tweak)
mise run changeset

# 2. Preview what the next release would do
mise run release:dry-run

# 3. Cut the release (bumps version, updates CHANGELOG, commits, tags locally)
mise run release

# 4. Push — triggers CI which builds binaries and updates the Homebrew formula
git push && git push --tags
```
