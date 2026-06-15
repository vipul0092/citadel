#!/usr/bin/env bash
# scripts/verify.sh — end-to-end smoke test for the citadel binary.
#
# Starts a headless server + client, exercises the control plane via
# citadel test, and asserts the expected events are received.
#
# Prerequisites:
#   mise run build     (produces ./citadel)
#
# Usage:
#   ./scripts/verify.sh
#   mise run verify

set -euo pipefail

BIN="./citadel"
TMP=$(mktemp -d)
SERVER_LOG="$TMP/server.jsonl"
PIDS=()

# ── helpers ──────────────────────────────────────────────────────────

log()  { printf '▶  %s\n' "$*"; }
pass() { printf '✓  %s\n' "$*"; }
fail() { printf '✗  %s\n' "$*" >&2; exit 1; }

cleanup() {
  [[ ${#PIDS[@]} -gt 0 ]] && kill "${PIDS[@]}" 2>/dev/null || true
  rm -rf "$TMP"
}
trap cleanup EXIT

# Poll ~/.citadel/run/*.json until a sentinel with matching role+name appears
# AND its control socket file exists on disk (guards against stale sentinels
# from a previous run that exited before this one starts).
# Prints the sentinel file path on success.
wait_sentinel() {
  local role=$1 name=$2 deadline=$((SECONDS + 15))
  while [[ $SECONDS -lt $deadline ]]; do
    for f in "${HOME}/.citadel/run/"*.json; do
      [[ -f "$f" ]] || continue
      r=$(python3 -c "import json; print(json.load(open('$f')).get('role',''))" 2>/dev/null) || continue
      n=$(python3 -c "import json; print(json.load(open('$f')).get('name',''))" 2>/dev/null) || continue
      [[ "$r" == "$role" && "$n" == "$name" ]] || continue
      sock=$(python3 -c "import json; print(json.load(open('$f')).get('control',''))" 2>/dev/null) || continue
      [[ -S "$sock" ]] && { echo "$f"; return 0; }
    done
    sleep 0.3
  done
  fail "timed out waiting for $role sentinel '$name'"
}

# Extract one top-level string field from a JSON file.
json_field() { python3 -c "import json; print(json.load(open('$1'))['$2'])"; }

# Poll a file until grep finds pattern. Timeout in seconds (default 8).
wait_for() {
  local pattern=$1 file=$2 deadline=$((SECONDS + ${3:-8}))
  while [[ $SECONDS -lt $deadline ]]; do
    grep -q "$pattern" "$file" 2>/dev/null && return 0
    sleep 0.2
  done
  fail "timed out waiting for '$pattern' in $(basename "$file")"
}

# ── 1. check binary ──────────────────────────────────────────────────

[[ -x "$BIN" ]] || fail "binary not found at $BIN — run 'mise run build' first"
log "binary OK: $("$BIN" version 2>&1 | head -1)"

# ── 2. start server (headless, OS-assigned port) ─────────────────────

log "starting server…"
"$BIN" server --name VerifyServer --port 0 --headless >/dev/null 2>&1 &
PIDS+=($!)

SENTINEL_S=$(wait_sentinel server VerifyServer)
SERVER_ADDR=$(json_field "$SENTINEL_S" addr)
SERVER_SOCK=$(json_field "$SENTINEL_S" control)
pass "server ready  addr=$SERVER_ADDR"

# ── 3. subscribe to server events before client connects ─────────────

"$BIN" test watch --sock "$SERVER_SOCK" --role server --level full \
  >"$SERVER_LOG" 2>/dev/null &
PIDS+=($!)
sleep 0.3   # let the subscribe handshake complete

# ── 4. start client (headless) ───────────────────────────────────────

log "starting client…"
"$BIN" connect --server "$SERVER_ADDR" --name VerifyClient --headless >/dev/null 2>&1 &
PIDS+=($!)

SENTINEL_C=$(wait_sentinel client VerifyClient)
CLIENT_SOCK=$(json_field "$SENTINEL_C" control)
pass "client ready"

# ── scenario 1: peer-join ────────────────────────────────────────────

log "scenario 1: peer-join"
wait_for '"ev":"peer-join"'    "$SERVER_LOG"
wait_for '"name":"VerifyClient"' "$SERVER_LOG"
pass "peer-join received on server"

# ── scenario 2: list-peers ───────────────────────────────────────────
# Background drive subscribes + sends list-peers; wait_for polls the
# output file. Cleanup kills the background process on exit.

log "scenario 2: list-peers"
PEERS_FILE="$TMP/peers.jsonl"
{
  printf '{"op":"subscribe","level":"summary","since":0}\n'
  printf '{"op":"list-peers"}\n'
  sleep 10
} | "$BIN" test drive --sock "$SERVER_SOCK" --role server \
  >"$PEERS_FILE" 2>/dev/null &
PIDS+=($!)
wait_for '"ev":"peers"' "$PEERS_FILE"
pass "list-peers returned peers snapshot"

# ── scenario 3: client chat ──────────────────────────────────────────

log "scenario 3: client chat"
"$BIN" test send-chat --sock "$CLIENT_SOCK" --text "hello from verify" >/dev/null 2>&1
wait_for '"ev":"chat"'           "$SERVER_LOG"
wait_for '"hello from verify"'   "$SERVER_LOG"
pass "chat event received on server"

# ── scenario 4: server say ───────────────────────────────────────────

log "scenario 4: server say"
"$BIN" test send-chat --sock "$SERVER_SOCK" --role server \
  --text "server says hello" >/dev/null 2>&1
wait_for '"ev":"say"' "$SERVER_LOG"
pass "say event received on server"

# ── scenario 5: kick ─────────────────────────────────────────────────

log "scenario 5: kick"
"$BIN" test kick --sock "$SERVER_SOCK" --name VerifyClient \
  --reason "verify script" >/dev/null 2>&1
wait_for '"ev":"kick"'             "$SERVER_LOG"
wait_for '"name":"VerifyClient"'   "$SERVER_LOG"
pass "kick event received on server"

# ── done ─────────────────────────────────────────────────────────────

echo ""
echo "✓  ALL CHECKS PASSED"
