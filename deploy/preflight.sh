#!/usr/bin/env bash
# Runs the full test suite locally. MUST pass before any deploy.
# Usage: bash deploy/preflight.sh
# Integration tests (dating flow vs real Postgres) run only when DATABASE_URL is set.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "==> backend: go vet"
(cd "$ROOT/backend" && go vet ./...)

echo "==> backend: go test $( [ -n "${DATABASE_URL:-}" ] && echo '(unit + integration: DATABASE_URL set)' || echo '(unit only — set DATABASE_URL for integration)' )"
(cd "$ROOT/backend" && go test ./...)

echo "==> frontend: unit tests"
(cd "$ROOT/frontend" && npm test)

echo "==> frontend: typecheck"
(cd "$ROOT/frontend" && npx tsc -b)

echo "==> frontend: lint"
(cd "$ROOT/frontend" && npm run lint)

echo "==> tgsidecar: pytest"
if [[ ! -d "$ROOT/tgsidecar/.venv" ]]; then
  (cd "$ROOT/tgsidecar" && python3 -m venv .venv && ./.venv/bin/pip install -q -r requirements-dev.txt)
fi
(cd "$ROOT/tgsidecar" && ./.venv/bin/python -m pytest -q)

echo "==> renderer: pytest (fit_text/build_html — no Chromium needed)"
if [[ ! -d "$ROOT/renderer/.venv" ]]; then
  (cd "$ROOT/renderer" && python3 -m venv .venv && ./.venv/bin/pip install -q pytest pydantic fastapi)
fi
(cd "$ROOT/renderer" && ./.venv/bin/python -m pytest -q test_generate.py)

echo "PREFLIGHT_OK"
