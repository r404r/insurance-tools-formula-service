# Implementation Log (实现履历)

## Phase 1: Foundation (基础框架)

### 2026-03-28: Project Initialization & Full Backend

**Status**: Complete

#### Backend - Completed
- [x] Go module initialized: `github.com/r404r/insurance-tools/formula-service/backend`
- [x] Domain models: Formula, FormulaGraph, FormulaNode, FormulaEdge, 8 NodeTypes, LookupTable
- [x] Version models: FormulaVersion, VersionState (draft/published/archived), User, Role, RBAC helpers
- [x] Config: env-based config with defaults (port 8080, SQLite, 28/18 precision, 24h JWT)
- [x] Store interfaces: FormulaRepository, VersionRepository, UserRepository, TableRepository
- [x] SQLite implementation: full CRUD, migrations, version state machine with transactions
- [x] Auth: JWT manager (HMAC-SHA256), RBAC (9 permissions x 4 roles), auth middleware
- [x] Calculation engine:
  - DAG builder with cycle detection
  - Kahn's algorithm topological sort → level-based parallel execution
  - Node evaluator (8 types) with shopspring/decimal precision
  - LRU result cache with SHA-256 input hashing
  - TableResolver for lookup table data loading
- [x] Parser: Pratt parser (7 precedence levels), lexer (21 token types), AST <-> DAG <-> text serializer, validator
- [x] API handlers: formula CRUD, versions, calculate, batch, validate, tables, users, auth
- [x] Router: chi with middleware (logger, recovery, CORS, auth, permission)
- [x] Entry point: main.go wires all subsystems with graceful shutdown

#### Backend - Code Review Fixes (via codex)
- [x] Fixed tableLookup nodes: added TableResolver interface + StoreTableResolver + preloadTableData
- [x] Fixed subFormula nodes: added evalSubFormula case in evaluator
- [x] Added missing table and user API routes with permission gating
- [x] Fixed admin registration race condition (create-then-promote pattern)

### 2026-03-28: Frontend Implementation

**Status**: Complete

#### Frontend - Completed
- [x] Vite + React 19 + TypeScript + Tailwind CSS 4 project setup
- [x] i18n: i18next with zh/ja/en translations
- [x] API client: fetch wrapper with JWT token management
- [x] Zustand stores: authStore (user/token/login/logout), formulaStore (editor state)
- [x] TypeScript types and utilities (graphSerializer, precisionFormat)
- [x] Auth: LoginPage, RegisterPage, ProtectedRoute
- [x] Layout: Navbar with language switcher, Layout with Outlet
- [x] FormulaList: search, domain filter tabs, create modal, table view
- [x] FormulaEditorPage: header, mode toggle (visual/text), save, test panel
- [x] FormulaCanvas: react-flow with drag-drop, auto-layout (dagre), node selection
- [x] NodePalette: draggable sidebar with 8 node types
- [x] NodePropertiesPanel: type-specific config editors
- [x] TextEditor: text expression editor with apply button
- [x] VersionsPage: version list with state badges, publish/archive actions
- [x] App.tsx: routing with Layout, ProtectedRoute, real components

### 2026-03-28: Build & DevOps

**Status**: Complete

- [x] Makefile: backend/frontend build, dev (parallel), test, clean, docker, migrate
- [x] docker-compose.yml: backend + frontend services with SQLite volume
- [x] Dockerfiles: multi-stage builds (backend: Go, frontend: Node + nginx)
- [x] .gitignore
- [x] CLAUDE.md
- [x] README.md
- [x] Frontend build verified (npm run build passes)
- [x] Backend build verified (go vet passes)

### Key Design Decisions

| Decision | Choice | Date |
|----------|--------|------|
| Formula storage format | JSON DAG (direct react-flow mapping) | 2026-03-28 |
| Editor mode | Dual mode (visual + text, bidirectional) | 2026-03-28 |
| Embedded DB | SQLite via modernc.org/sqlite (pure Go) | 2026-03-28 |
| Precision library | shopspring/decimal | 2026-03-28 |
| Parallel execution | Level-based topological dispatch + errgroup | 2026-03-28 |
| Version storage | Full snapshots (not deltas) | 2026-03-28 |
| Auth | JWT + RBAC (4 roles) | 2026-03-28 |
| i18n | i18next (zh/ja/en) | 2026-03-28 |
| Frontend state | Zustand + TanStack Query | 2026-03-28 |
| HTTP router | chi | 2026-03-28 |

---

## Pending Work

- [ ] PostgreSQL Store implementation
- [ ] MySQL Store implementation
- [ ] Custom react-flow node components (OperatorNode, VariableNode, etc.)
- [ ] Version diff view
- [ ] Insurance domain templates (life/property/auto)
- [ ] Lookup table management UI
- [ ] E2E testing
- [ ] Load testing
