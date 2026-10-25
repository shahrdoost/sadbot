#!/bin/bash
# Builds and starts the full stack on the server. Run from /opt/sadbot.
set -euo pipefail
cd /opt/sadbot

echo "==> docker compose build"
docker compose -f docker-compose.prod.yml --env-file .env build

echo "==> docker compose up"
docker compose -f docker-compose.prod.yml --env-file .env up -d

echo "==> waiting for backend health"
for i in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:8085/api/health >/dev/null 2>&1; then
    echo "HEALTH_OK"
    break
  fi
  sleep 2
done

docker compose -f docker-compose.prod.yml --env-file .env ps
