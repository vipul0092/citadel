# Security

## Current model: open / LAN-trusted

Anyone on the same LAN can connect to the server. The only gate is a **unique display name** — if a name is already taken, the server rejects the new connection with `reject{reason:"name taken"}`.

This is appropriate for:
- Home LAN / personal Wi-Fi where all devices are trusted
- LAN parties among friends

It is **not** appropriate for:
- Corporate networks (other users could connect)
- Any network where you don't trust all participants
- Internet exposure (no NAT traversal exists anyway)

## Future: shared passcode

Lowest-effort upgrade. Server prints a random passcode on startup (or accepts `--passcode`). Client adds a `passcode` field to the `hello` payload. Server rejects mismatches before checking name uniqueness.

Protocol change is backward-compatible: a version-1 server ignores the `passcode` field; a version-2 server requires it. The `hello` payload already carries `version`, so a clean negotiation is possible.

## Future: TLS + TOFU

Server generates a self-signed cert on first run (stored in `~/.citadel/<name>/cert.pem`). Clients connect over TLS and pin the server's fingerprint on first connect (Trust On First Use), stored in `~/.citadel/known_servers`. Subsequent connections verify the fingerprint and warn if it changes (analogous to SSH `known_hosts`).

This protects against a rogue server impersonating a trusted one on the same LAN.

## Non-goals

- End-to-end encryption of chat messages (server sees all content)
- Cross-internet play (no NAT traversal or relay)
- Persistent user accounts / password storage
