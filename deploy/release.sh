#!/usr/bin/env bash
# One-command release: preflight (full test suite) → rsync → server deploy.
# The deploy NEVER runs if tests fail, and the Docker build re-runs unit tests
# as a second gate on the server.
#
# Required env:
#   SSHPASS                 — SSH password for root@212.87.199.140 (via sshpass -e)
# Optional env:
#   SADBOT_SERVER           — default root@212.87.199.140
#   SADBOT_SOCKS            — default 127.0.0.1:10808
#   SADBOT_SKIP_PREFLIGHT   — set to "1" ONLY for emergencies (not recommended)
#
# Usage: SSHPASS=... bash deploy/release.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVER="${SADBOT_SERVER:-root@212.87.199.140}"
SOCKS="${SADBOT_SOCKS:-127.0.0.1:10808}"

if [[ -z "${SSHPASS:-}" ]]; then
  echo "ERROR: set SSHPASS (the SSH password for $SERVER)" >&2
  exit 1
fi

SSH_CMD=(sshpass -e ssh
  -o PreferredAuthentications=password -o PubkeyAuthentication=no
  -o StrictHostKeyChecking=accept-new
  -o "ProxyCommand=nc -X 5 -x ${SOCKS} %h %p")

# ---- gate 1: full local test suite ----
if [[ "${SADBOT_SKIP_PREFLIGHT:-0}" != "1" ]]; then
  bash "$ROOT/deploy/preflight.sh"
else
  echo "WARNING: SADBOT_SKIP_PREFLIGHT=1 — skipping local tests!" >&2
fi

# ---- ship the tree ----
# rsync -e runs its argument through a subshell, and the ProxyCommand (spaces)
# breaks when flattened into one string. Point -e at a wrapper file instead.
WRAPPER="$(mktemp -t sb-ssh)"
chmod 700 "$WRAPPER"
{
  echo '#!/bin/bash'
  printf 'exec'
  printf ' %q' "${SSH_CMD[@]}"
  printf ' "$@"\n'
} > "$WRAPPER"
trap 'rm -f "$WRAPPER"' EXIT

echo "==> rsync to $SERVER:/opt/sadbot/"
rsync -az -e "$WRAPPER" \
  --exclude .git --exclude node_modules --exclude dist \
  --exclude kpkchat --exclude .env --exclude '*/pgdata' \
  --exclude '*/.venv' --exclude 'renderer/out' \
  "$ROOT/" "$SERVER:/opt/sadbot/"

# ---- gate 2: server build re-runs unit tests inside Docker, then up + health ----
echo "==> running deploy.sh on server"
"${SSH_CMD[@]}" "$SERVER" 'cd /opt/sadbot && bash deploy/deploy.sh'

echo "RELEASE_OK — panel: http://212.87.199.140:8085"
