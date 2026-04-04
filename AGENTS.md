# AGENTS.md

## Project Overview

Insurance formula calculation engine with visual editor, high-precision computation, and multi-database support.

## Quick Start

```bash
# Development (backend + frontend in parallel)
make dev

# Build both
make build

# Docker
docker compose up -d
```

## Project Structure

- `backend/` — Go API service (chi router, shopspring/decimal precision, SQLite/PostgreSQL/MySQL)
- `frontend/` — React + TypeScript + Tailwind + @xyflow/react visual editor
- `docs/` — Requirements, design docs, implementation log

## Backend

- **Module**: `github.com/r404r/insurance-tools/formula-service/backend`
- **Entry**: `backend/cmd/server/main.go`
- **Build**: `cd backend && go build -o bin/server ./cmd/server`
- **Test**: `cd backend && go test ./... -v`
- **Lint**: `cd backend && go vet ./...`

Key packages:
- `internal/engine/` — DAG-based calculation engine with parallel execution
- `internal/parser/` — Pratt parser for text expressions (AST <-> DAG <-> text)
- `internal/store/sqlite/` — SQLite repository (primary)
- `internal/api/` — HTTP handlers and routing
- `internal/auth/` — JWT + RBAC (admin/editor/reviewer/viewer)
- `internal/domain/` — Domain models

## Frontend

- **Build**: `cd frontend && npm run build`
- **Dev**: `cd frontend && npm run dev`
- API proxy configured in `vite.config.ts` → localhost:8080

Key paths:
- `src/components/editor/` — Visual + text formula editor
- `src/components/version/` — Version management
- `src/components/auth/` — Login/register
- `src/i18n/locales/` — zh/ja/en translations
- `src/store/` — Zustand stores (auth, formula)

## Config

Backend reads env vars:
- `SERVER_PORT` (default 8080)
- `DB_DRIVER` (sqlite/postgres/mysql)
- `DB_DSN` (connection string)
- `AUTH_JWT_SECRET`
- `ENGINE_INTERMEDIATE_PRECISION` (default 28)
- `ENGINE_OUTPUT_PRECISION` (default 18)

## Conventions

- Formula data model is JSON DAG (nodes + edges + outputs)
- High-precision decimals via shopspring/decimal; results returned as strings
- i18n keys follow `section.key` pattern (e.g., `formula.name`, `editor.save`)
- API routes under `/api/v1/`
