# Discovery

Citadel uses three complementary discovery mechanisms tried in parallel. All of them are transparent to the user — `citadel connect` (no flags) handles everything automatically.

---

## How `citadel connect` works (no flags)

```
citadel connect
    │
    ├─ immediately ──────────────────────────────────────────────────────────
    │   Probe localhost:7777 (300 ms timeout)
    │   → success: jump straight to name prompt  (<300 ms, same-machine)
    │   → failure: continue silently
    │
    ├─ in parallel ──────────────────────────────────────────────────────────
    │   mDNS / Zeroconf browse  (_citadel._tcp)
    │   UDP broadcast listen    (port 7778)
    │
    └─ as results arrive ────────────────────────────────────────────────────
        0 servers  → "Scanning…"
        1 server   → auto-connect countdown: 3 s (any key to cancel)
        2+ servers → interactive picker (↑/↓ navigate, Enter connect)
        after 5 s with 0 results → show hint, keep scanning indefinitely
```

The client never gives up scanning on its own. If auto-discovery doesn't work, the hint message tells the user to use `--server`.

---

## Mechanism 1 — Localhost probe (same-machine fast path)

Dials `localhost:7777` with a 300 ms timeout immediately on startup. If it succeeds, the user goes straight to the name prompt — no scan delay. This exists because mDNS loopback is unreliable on macOS, so the two-machine discovery stack would add unnecessary latency on the common same-machine-dev case.

---

## Mechanism 2 — UDP directed broadcast (primary cross-machine)

This is the mechanism that makes `citadel connect` work on typical home WiFi.

**Why not `255.255.255.255`?**  
`255.255.255.255` is the *limited broadcast* address. Despite the name, most WiFi drivers deliver it only to the sending machine — it is never forwarded to other devices, even on the same network. Using it produces silently broken discovery.

**Directed broadcast instead**  
On startup, the server enumerates its active network interfaces and computes the *directed broadcast* address for each subnet — the address with all host bits set (e.g. `192.168.1.255` for a `/24` network). This address is forwarded by virtually all home routers to every device on that subnet, even when mDNS multicast is blocked by AP isolation.

```
Server (192.168.1.5)                Client (192.168.1.8)
  │                                   │
  ├── every 2 s ──────────────────►   │  CITADEL:Throne:7777
  │   UDP → 192.168.1.255:7778        │  (directed broadcast)
  │                                   ├─ sender IP = 192.168.1.5
  │                                   ├─ port from payload = 7777
  │                                   └─ ServerInfo{Throne, 192.168.1.5:7777}
```

The server sends to every interface's directed broadcast simultaneously, so multi-homed machines (WiFi + Ethernet) work correctly.

**Packet format**: `CITADEL:<name>:<ip>:<port>` — the IP is the server's primary outbound interface (from `LocalIPv4()`), not the broadcast sender address. This ensures the client always dials the right interface on multi-homed machines (e.g. Ethernet + WiFi active simultaneously).  
**Server port**: 7778 UDP (fixed, not configurable)  
**Interval**: every 2 seconds

---

## Mechanism 3 — mDNS / Zeroconf

The server registers `_citadel._tcp` on the local domain via `github.com/grandcat/zeroconf`. TXT records carry `name=<name>` and `version=1`. The client browses the same service type.

**Limitation**: mDNS uses multicast UDP (`224.0.0.251`). Many routers and all managed/commercial WiFi access points enable **AP isolation** (client isolation), which blocks multicast between wireless clients. When AP isolation is active, mDNS finds nothing and UDP broadcast takes over.

mDNS is kept because:
- It works on some enterprise networks where broadcast is blocked
- It carries richer metadata (TXT records)
- It's standard and worth retaining

---

## Mechanism 4 — Manual `--server host:port`

Bypasses all discovery entirely. Use when:
- Both mDNS and broadcast fail (separate subnets, VPN, Docker networks)
- You already know the address
- Scripting / CI

The server TUI header always shows the LAN address so you can share it.

```sh
citadel connect --server 192.168.1.5:7777
```

---

## Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 7777 | TCP | Main service (configurable via `--port`) |
| 7778 | UDP | Broadcast presence (fixed) |
| 5353 | UDP | mDNS (system-managed, shared with OS mDNSResponder/Avahi) |

Port 7778 must be reachable between machines (not blocked by firewall) for broadcast discovery to work.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|-------------|-----|
| `citadel connect` scans forever, no server found | AP isolation active + broadcast blocked by firewall | Use `--server <ip>:7777` |
| Works locally, not cross-machine | mDNS loopback issue | Expected — broadcast handles cross-machine |
| Cross-machine connect with broadcast doesn't work | OS firewall blocking UDP 7778 | Allow Citadel through the firewall, or use `--server` |
| Server appears but can't connect | Wrong version, or TCP port 7777 is firewalled | Check server version matches client; allow TCP 7777 |
