# Implementation Log (实现履历)

## Phase 1: Foundation (基础框架)

### 2026-03-28: Project Initialization

**Status**: In Progress

#### Completed
- [x] Go module initialized: `github.com/r404r/insurance-tools/formula-service/backend`
- [x] Backend directory structure created:
  ```
  backend/
  ├── cmd/server/
  ├── internal/
  │   ├── engine/     (calculation engine)
  │   ├── parser/     (formula parser)
  │   ├── api/        (HTTP handlers)
  │   ├── auth/       (RBAC + JWT)
  │   ├── store/      (repository + SQLite/PG/MySQL)
  │   ├── domain/     (models + insurance domains)
  │   └── config/     (configuration)
  └── pkg/decimal/
  ```
- [x] Domain models implemented:
  - `domain/formula.go`: Formula, FormulaGraph, FormulaNode, FormulaEdge, NodeType, LookupTable, config types
  - `domain/version.go`: FormulaVersion, VersionState, User, Role, RBAC helpers, VersionDiff

#### In Progress
- [ ] Store interfaces + SQLite backend implementation
- [ ] Calculation engine (DAG, evaluator, parallel executor)
- [ ] Formula parser (AST, lexer, Pratt parser, serializer)
- [ ] Auth system (RBAC, JWT, middleware)
- [ ] API layer (router, handlers, middleware)
- [ ] Config management

#### Pending
- [ ] Frontend initialization (Vite + React + TypeScript + Tailwind)
- [ ] Frontend i18n setup (zh/ja/en)
- [ ] Frontend formula editor (react-flow)
- [ ] Frontend version management UI
- [ ] Frontend auth pages
- [ ] Makefile, .gitignore, docker-compose
- [ ] PostgreSQL Store implementation
- [ ] MySQL Store implementation

### Key Design Decisions Made

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

## Phase 2: Calculation Engine

*Not yet started*

## Phase 3: Visual Editor

*Not yet started*

## Phase 4: Version Management UI

*Not yet started*

## Phase 5: Insurance Domains

*Not yet started*

## Phase 6: Production Hardening

*Not yet started*
