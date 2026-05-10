# ADR 0004 — Discovery: localhost probe + mDNS + UDP directed broadcast + manual fallback

## Context

Clients need to find the server on the LAN automatically without flags.

This ADR documents the full evolution of discovery, including two bugs discovered during testing.

---

## Round 1 — mDNS only (original design)

**Decision**: use `grandcat/zeroconf` to register `_citadel._tcp` on the server and browse on the client.

**What went wrong**:
1. **Same-machine**: `grandcat/zeroconf` uses raw multicast sockets that don't loop back to the same process on macOS. `citadel connect` on the same machine as the server found nothing.
2. **Cross-machine, home WiFi**: mDNS uses multicast UDP (`224.0.0.251`). Most home routers and all managed/commercial access points enable **AP isolation** (client isolation) by default, which drops multicast between wireless clients. Cross-machine mDNS found nothing.

---

## Round 2 — Add UDP broadcast to `255.255.255.255`

**Decision**: server sends `CITADEL:<name>:<port>` to `255.255.255.255:7778` every 2 s; client listens on port 7778.

**What went wrong**: `255.255.255.255` is the *limited broadcast* address. Despite the name, most WiFi drivers deliver it **only to the sending machine** — it is never forwarded to other devices on the same network. Cross-machine discovery still didn't work.

---

## Round 3 — Switch to UDP directed broadcast (current)

**Decision**: enumerate all active network interfaces on the server, compute the **directed broadcast** address for each subnet (all host bits set — e.g. `192.168.1.255` for `/24`), and send to all of them every 2 s.

**Why directed broadcast works**: this address is forwarded by virtually all home routers to every device on the subnet, even when mDNS multicast and AP isolation block other mechanisms. Confirmed working in testing between two machines on a home WiFi network that had AP isolation active.

Also added:
- **Localhost probe** (300 ms timeout) fired immediately on `citadel connect`, as a fast path for same-machine use that bypasses both mDNS and broadcast.
- **No-give-up scanning**: the client no longer terminates scanning after a timeout. Instead, after 5 s with no results it shows a hint and keeps scanning indefinitely. If broadcast starts working (e.g. firewall rule added), the server appears without restarting.

---

## Final decision (what's in the code)

Four layers, tried in parallel, first win:

| Layer | Mechanism | Latency | Works when |
|-------|-----------|---------|-----------|
| 1 | Localhost probe (300 ms TCP) | < 300 ms | Server on same machine |
| 2 | UDP directed broadcast | ≤ 2 s | Same subnet, any router |
| 3 | mDNS / Zeroconf | 1–3 s | Multicast not blocked |
| 4 | `--server host:port` flag | instant | Always |

## Consequences

- `citadel connect` works on same machine, typical home WiFi, and explicitly-addressed scenarios.
- UDP port 7778 must not be blocked by host firewalls between machines.
- `255.255.255.255` is never used — documented pitfall for future reference.
- mDNS is retained as a complement; it's not the primary mechanism.

## Key lessons

**`255.255.255.255` is OS-local**: the limited broadcast address does not reach other machines on most WiFi stacks. Always use the subnet's directed broadcast address (computed from interface + mask) for cross-machine UDP broadcast.

**Multi-homed servers**: a machine with multiple active network interfaces (e.g. Ethernet + WiFi) broadcasts from all of them. The client receives whichever packet arrives first, and the sender IP may be an interface that is not reachable from the client (e.g. a cable that goes nowhere). Fix: embed the server's **primary outbound IP** (via `LocalIPv4()` — the IP used to route toward the internet) in the broadcast payload rather than relying on the sender IP. The client uses the payload IP to dial, ignoring which physical interface delivered the packet.
