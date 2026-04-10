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

1. Reload and Check `docs/backlog.md` for current priorities
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
>
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

### Task Completion Self-Check

停止工作前，验证以下各项：

- [ ] **Task 文件存在**：`docs/tasks/NNN-slug.md`，Status 为 in-progress（或刚改为 done）
- [ ] **backlog.md 已更新**：如 task 完成，已移至「已完成」
- [ ] **codex review 已执行**：每次 `git commit` 前都运行过 `/codex review`
- [ ] **无遗漏变更**：`git status` 确认所有变更已提交
- [ ] **中断记录已写**：如中途停止，task 文件已更新 TODO 勾选和中断记录

> **Hook 强制执行**：`git commit` 会被 PreToolUse hook 拦截，当：
>
> 1. `docs/tasks/` 中无 Status 为 in-progress 的 task 文件
> 2. 上次 commit 后未运行 `codex review`

## Testing

### Test Directory Structure

```
tests/
├── screenshots/         # Visual verification screenshots (by task number)
│   ├── 020/            # Task #020 screenshots
│   └── 021/            # Task #021 screenshots
├── batch/              # Batch test data (JSON, reusable for regression)
│   └── 023/            # Task #023 batch tests
│       ├── pure-premium-30cases.json
│       ├── annuity-30cases.json
│       └── reserve-30cases.json
└── reports/            # Test reports (Markdown)
    └── 023-life-insurance-report.md
```

### Batch Test Data Format

```json
[
  {"label": "case-01", "inputs": {"x": "30", "n": "20"}, "expected": {"result": "1.234"}}
]
```

### Rules

- 每个 task 的截图保存到 `tests/screenshots/{task-number}/`
- 批量测试数据保存到 `tests/batch/{task-number}/`，JSON 格式，可重复执行
- 测试报告保存到 `tests/reports/`，Markdown 格式
- 批量测试数据同时作为回归测试 case 保留

## Prompt History

每次接收到用户新的提示词时，**第一动作**是将该提示词原文落盘保存到 `prompt_history/` 目录，然后再开始处理。

### 规则

- 目录：`prompt_history/`
- 文件名：`YYYY-MM-DD.md`（按日期分文件）
- 格式：Markdown，每个 prompt 用 `## Prompt N` 标题分隔
- 内容：用户提示词的原文（包括标点符号、换行）
- 时机：**收到新提示词后立即保存**，不要等到任务完成
- 同日多个提示词：追加到当天的文件末尾，编号递增

### 示例

```markdown
# Prompt History — 2026-04-10

## Prompt 1
用户的第一条提示词原文...

## Prompt 2
用户的第二条提示词原文...
```

> **注意**：仅保存用户的原始输入文本，不要加注释、解释或修改。这是历史记录，不是工作日志。
