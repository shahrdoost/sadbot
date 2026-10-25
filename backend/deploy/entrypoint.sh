#!/bin/sh
set -e

echo "[entrypoint] waiting for database..."
i=0
until pg_isready -h "${DB_HOST:-db}" -p "${DB_PORT:-5432}" -U "${DB_USER:-sadbot}" 2>/dev/null || [ $i -ge 30 ]; do
  i=$((i+1))
  sleep 1
done

# run migrations with our bundled migrate binary if present
if [ "${RUN_MIGRATIONS:-1}" = "1" ] && [ -n "$DATABASE_URL" ]; then
  echo "[entrypoint] running migrations..."
  /app/migrate || exit 1
fi

echo "[entrypoint] starting sadbot on ${ADDR:-:8080}"
exec /app/sadbot
