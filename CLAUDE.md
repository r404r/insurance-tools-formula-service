# CLAUDE.md

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

## Development Workflow

### Before Starting Any Task
1. Check `docs/backlog.md` for current priorities
2. Check `docs/tasks/` for any in-progress task (Status: in-progress)
3. If resuming interrupted work, read the task file's "中断记录" section first

### For Each New Feature/Fix
1. Create task file: `docs/tasks/NNN-slug.md` using template in `docs/tasks/TEMPLATE.md`
2. Fill in 需求 and 设计 sections, get user confirmation
3. Break down into TODO checklist
4. Implement step by step, marking each TODO as done
5. When feature is complete: run `/codex review`, fix any P1/P2 findings, then commit
6. Update `docs/backlog.md`: move task to 已完成

> **强制执行提醒（曾违反过）：**
> - 步骤 1（task 文件）和步骤 5（codex review）是**硬性前提条件**，不得跳过。
>   无论需求多小、多紧急，没有 task 文件 = 不能开始实现；没有 codex review = 不能 commit。
> - 同一个会话内连续实现多个功能时，**每个功能**都要独立走完上述步骤，不能批量合并处理。
> - 过去曾发生：在用户要求的任务（006 并发控制）完成后，立即被追加了新需求（007 管理员设置页），
>   直接开始实现而未先创建 task 文件、也未在 commit 前执行 codex review，导致事后补救。

### On Interruption
Before ending a session or when token is running low:
- Update the task file's TODO checkmarks to reflect actual progress
- Write "中断记录" section: what was just done, what's next, any context needed

### On Resume
1. Read `docs/backlog.md` to see overall status
2. Find task with Status: in-progress
3. Read its 中断记录 and TODO list
4. Continue from where it left off

### Task Numbering
- Sequential: 001, 002, 003...
- Check existing files in `docs/tasks/` to determine next number
