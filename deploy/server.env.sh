#!/bin/bash
# Generates /opt/sadbot/.env on the server with strong random secrets.
set -euo pipefail

ENV_FILE=/opt/sadbot/.env

PG_PASSWORD=$(openssl rand -hex 18)
JWT_SECRET=$(openssl rand -hex 32)
TOKEN_KEY=$(openssl rand -hex 32)
ADMIN_PASSWORD=$(openssl rand -hex 9)
SIDECAR_KEY=$(openssl rand -hex 24)

umask 077
cat > "$ENV_FILE" <<EOF
POSTGRES_DB=sadbot
POSTGRES_USER=sadbot
POSTGRES_PASSWORD=$PG_PASSWORD
JWT_SECRET=$JWT_SECRET
TOKEN_ENCRYPTION_KEY=$TOKEN_KEY
SEED_ADMIN_EMAIL=admin@sadbot.local
SEED_ADMIN_PASSWORD=$ADMIN_PASSWORD
EVENT_RETENTION_DAYS=90
TGSIDECAR_KEY=$SIDECAR_KEY
# MTProto app credentials (from my.telegram.org) for the tgsidecar service
TG_API_ID=37257760
TG_API_HASH=766bc4e2116018f13ea5da45986c2df7
# leave empty unless Telegram is blocked from the server
TG_PROXY=
EOF

echo "ENV_WRITTEN"
echo "ADMIN_EMAIL=admin@sadbot.local"
echo "ADMIN_PASSWORD=$ADMIN_PASSWORD"
