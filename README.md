# Insurance Formula Service

A visual formula calculation engine for the insurance industry, supporting life insurance, property insurance, and auto insurance domains.

## Features

- **Visual Formula Editor** — Drag-and-drop DAG editor powered by React Flow, with 8 node types (variable, constant, operator, function, subFormula, tableLookup, conditional, aggregate)
- **Dual-Mode Editing** — Switch between visual canvas and text expression mode with bidirectional conversion via Pratt parser
- **High-Precision Computation** — Financial-grade decimal arithmetic (18-28 decimal places) using shopspring/decimal
- **Parallel Execution** — Automatic DAG-based parallelization of independent computation branches
- **Version Management** — Draft → Published → Archived state machine with full snapshot versioning
- **Multi-Database** — Repository pattern supporting SQLite (embedded), PostgreSQL, and MySQL
- **RBAC** — JWT-based authentication with Admin, Editor, Reviewer, and Viewer roles
- **i18n** — Chinese, Japanese, and English localization

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Frontend | React 19, TypeScript, Tailwind CSS 4, @xyflow/react 12 |
| State | Zustand, TanStack Query |
| Backend | Go 1.26, chi router |
| Precision | shopspring/decimal |
| Database | SQLite (modernc.org/sqlite), PostgreSQL (pgx), MySQL |
| Auth | golang-jwt, bcrypt |

## Quick Start

### Development

```bash
# Install frontend dependencies
make frontend-install

# Start backend + frontend dev servers
make dev
```

Backend runs on `http://localhost:8080`, frontend on `http://localhost:5173` with API proxy.

### Docker

```bash
docker compose up -d
```

Backend on port 8080, frontend on port 3000.

### Build

```bash
make build
```

## API

All endpoints under `/api/v1/`:

| Method | Path | Description |
|--------|------|-------------|
| POST | `/auth/login` | Login |
| POST | `/auth/register` | Register (first user becomes admin) |
| GET | `/formulas` | List formulas (filter by domain, search) |
| POST | `/formulas` | Create formula |
| GET | `/formulas/:id` | Get formula |
| PUT | `/formulas/:id` | Update formula |
| DELETE | `/formulas/:id` | Delete formula |
| GET | `/formulas/:id/versions` | List versions |
| POST | `/formulas/:id/versions` | Create version |
| PATCH | `/formulas/:id/versions/:ver` | Update version state |
| POST | `/calculate` | Execute calculation |
| POST | `/calculate/batch` | Batch calculation |
| POST | `/calculate/validate` | Validate formula |

## Formula Model

Formulas are stored as JSON DAGs:

```json
{
  "nodes": [
    {"id": "n1", "type": "variable", "config": {"name": "age", "dataType": "integer"}},
    {"id": "n2", "type": "operator", "config": {"op": "multiply"}},
    {"id": "n3", "type": "function", "config": {"fn": "round", "args": {"places": 18}}}
  ],
  "edges": [
    {"source": "n1", "target": "n2", "sourcePort": "out", "targetPort": "left"},
    {"source": "n2", "target": "n3", "sourcePort": "out", "targetPort": "in"}
  ],
  "outputs": ["n3"]
}
```

## Project Structure

```
formula-service/
├── backend/
│   ├── cmd/server/         # Entry point
│   └── internal/
│       ├── api/            # HTTP handlers
│       ├── auth/           # JWT + RBAC
│       ├── config/         # Configuration
│       ├── domain/         # Domain models
│       ├── engine/         # Calculation engine (DAG, parallel, evaluator)
│       ├── parser/         # Pratt parser (AST, lexer, serializer)
│       └── store/sqlite/   # Database layer
├── frontend/
│   └── src/
│       ├── api/            # API client
│       ├── components/     # React components
│       ├── i18n/           # Translations (zh/ja/en)
│       ├── store/          # Zustand stores
│       ├── types/          # TypeScript types
│       └── utils/          # Serializers, formatters
├── docs/                   # Requirements, design, implementation log
├── Makefile
└── docker-compose.yml
```

## License

Private
