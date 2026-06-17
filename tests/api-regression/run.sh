#!/usr/bin/env bash
#
# Reusable API regression run.
#
# Drives a running backend through the core API surface (see
# backend/cmd/api_regression) and writes a Markdown report to tests/reports/.
# The default target is the PostgreSQL-backed containerized stack, matching the
# project's default database. Set BASE_URL to point at an already-running
# backend (any driver) and the script skips Docker entirely.
#
# Usage:
#   tests/api-regression/run.sh                 # bring up postgres stack, seed, run
#   BASE_URL=http://localhost:8080 tests/api-regression/run.sh   # use running backend
#   tests/api-regression/run.sh --no-seed       # skip the seed-runner step
#   tests/api-regression/run.sh --down          # tear the stack down when finished
#
# Environment:
#   BASE_URL        backend base URL (default http://localhost:8080)
#   COMPOSE_PROFILE docker compose profile to start (default postgres)
#   ADMIN_USER      admin username for login (default admin)
#   ADMIN_PASS      admin password for login (default admin99999)
#
set -euo pipefail

# Resolve repo root (this script lives at tests/api-regression/run.sh).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

BASE_URL="${BASE_URL:-http://localhost:8080}"
COMPOSE_PROFILE="${COMPOSE_PROFILE:-postgres}"
REPORT_DIR="${REPORT_DIR:-$REPO_ROOT/tests/reports}"
DO_SEED=1
DO_DOWN=0
MANAGE_STACK=0

for arg in "$@"; do
  case "$arg" in
    --no-seed) DO_SEED=0 ;;
    --down)    DO_DOWN=1 ;;
    -h|--help) sed -n '2,30p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "unknown argument: $arg" >&2; exit 2 ;;
  esac
done

healthy() { curl -fsS -o /dev/null "$BASE_URL/healthz" 2>/dev/null; }

wait_healthy() {
  local tries="${1:-60}"
  for _ in $(seq 1 "$tries"); do
    if healthy; then return 0; fi
    sleep 2
  done
  return 1
}

cleanup() {
  if [[ "$MANAGE_STACK" == "1" && "$DO_DOWN" == "1" ]]; then
    echo "==> Tearing down docker stack"
    docker compose --profile "$COMPOSE_PROFILE" --profile seed down
  fi
}
trap cleanup EXIT

# If nothing is already serving, bring up the containerized stack.
if healthy; then
  echo "==> Backend already healthy at $BASE_URL — skipping Docker"
else
  if ! command -v docker >/dev/null 2>&1; then
    echo "ERROR: no backend at $BASE_URL and docker is not available." >&2
    echo "       Start a backend manually or set BASE_URL." >&2
    exit 1
  fi
  MANAGE_STACK=1
  echo "==> Starting docker compose (profile=$COMPOSE_PROFILE)"
  docker compose --profile "$COMPOSE_PROFILE" up -d --build

  echo "==> Waiting for $BASE_URL/healthz"
  if ! wait_healthy 60; then
    echo "ERROR: backend did not become healthy in time." >&2
    docker compose --profile "$COMPOSE_PROFILE" logs --tail 50 || true
    exit 1
  fi

  if [[ "$DO_SEED" == "1" ]]; then
    echo "==> Seeding sample data (idempotent)"
    docker compose --profile "$COMPOSE_PROFILE" --profile seed run --rm seed-runner || \
      echo "WARN: seed-runner failed or already seeded — continuing"
  fi
fi

echo "==> Running API regression suite against $BASE_URL"
mkdir -p "$REPORT_DIR"
set +e
BASE_URL="$BASE_URL" ADMIN_USER="${ADMIN_USER:-admin}" ADMIN_PASS="${ADMIN_PASS:-admin99999}" \
  REPORT_DIR="$REPORT_DIR" \
  bash -c 'cd "$1/backend" && go run ./cmd/api_regression' _ "$REPO_ROOT"
RC=$?
set -e

echo "==> Report written under $REPORT_DIR (api-regression-latest.md)"
exit $RC
