#!/usr/bin/env bash
# Task #018 — Master Test Runner
# Usage: ./run.sh [--api-only] [--unit-only]
#
# Full run (default):
#   1. Frontend vitest unit tests
#   2. Playwright E2E tests (requires dev server + backend running)
#   3. Generate markdown report
#
# Flags:
#   --api-only   Skip UI screenshots, run only API calculation tests
#   --unit-only  Run only vitest unit tests

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
FRONTEND="$ROOT/frontend"

API_ONLY=false
UNIT_ONLY=false

for arg in "$@"; do
  case $arg in
    --api-only) API_ONLY=true ;;
    --unit-only) UNIT_ONLY=true ;;
  esac
done

echo "========================================"
echo " Task #018 LaTeX Test Suite"
echo "========================================"

# ── Step 1: Vitest unit tests ────────────────────────────────────────────────

echo ""
echo "▶ Step 1: Running vitest unit tests..."
(cd "$FRONTEND" && npm test -- --reporter=verbose 2>&1 | grep -E "✓|×|PASS|FAIL|Tests|Test Files") || {
  echo "❌ Unit tests failed"
  exit 1
}
echo "✅ Unit tests passed"

if $UNIT_ONLY; then
  echo ""
  echo "Unit-only mode: done."
  exit 0
fi

# ── Step 2: Install Playwright if needed ─────────────────────────────────────

echo ""
echo "▶ Step 2: Checking Playwright..."
cd "$SCRIPT_DIR"
if [ ! -d "node_modules" ]; then
  echo "Installing Playwright dependencies..."
  npm install
  npx playwright install chromium
fi

# ── Step 3: E2E tests ────────────────────────────────────────────────────────

echo ""
echo "▶ Step 3: Running Playwright E2E tests..."
echo "  (requires: backend on :8080, frontend on :5173)"
echo ""

PLAYWRIGHT_ARGS=""
if $API_ONLY; then
  # Pass env var to skip UI navigation steps
  export SKIP_UI=true
fi

npx playwright test --config=playwright.config.ts $PLAYWRIGHT_ARGS || {
  echo ""
  echo "⚠️  Some E2E tests failed. Check output above."
  echo "     Generating report with partial results..."
}

# ── Step 4: Generate report ──────────────────────────────────────────────────

echo ""
echo "▶ Step 4: Generating test report..."
node generate-report.mjs

echo ""
echo "========================================"
echo " Done! Report: tests/reports/018-latex-test-report.md"
echo "========================================"
