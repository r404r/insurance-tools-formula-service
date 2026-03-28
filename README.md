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

## Default Account & Seed Data

On first startup, the system automatically creates:

### Default Admin Account

| Field | Value |
|-------|-------|
| Username | `admin` |
| Password | `admin99999` |
| Role | Admin |

### Seed Formulas

The system includes three pre-built insurance formulas, each with a published v1:

#### 1. Life Insurance: Net Premium (寿险净保费计算)

Formula: `premium = sumAssured × qx × v`, where `v = 1 / (1 + interestRate)`

| Input | Description | Example |
|-------|-------------|---------|
| `sumAssured` | Sum assured (保额) | 1000000 |
| `qx` | Mortality rate (死亡率) | 0.001 |
| `interestRate` | Interest rate (预定利率) | 0.035 |

Example result: `966.183574879227053140` (18-digit precision)

#### 2. Property Insurance: Premium Rating (财产险保费计算)

Formula: `premium = baseRate × riskScore × sumInsured × (1 - discount)`

| Input | Description | Example |
|-------|-------------|---------|
| `baseRate` | Base rate (基础费率) | 0.003 |
| `riskScore` | Risk score (风险评分) | 1.2 |
| `sumInsured` | Sum insured (保额) | 5000000 |
| `discount` | Discount rate (折扣率) | 0.1 |

#### 3. Auto Insurance: Commercial Premium (车险商业保费计算)

Formula: `premium = basePremium × vehicleFactor × driverFactor × ncdDiscount`

| Input | Description | Example |
|-------|-------------|---------|
| `basePremium` | Base premium (基础保费) | 3000 |
| `vehicleFactor` | Vehicle factor (车辆系数) | 1.1 |
| `driverFactor` | Driver risk factor (驾驶员系数) | 0.95 |
| `ncdDiscount` | No-claim discount (无赔优惠系数) | 0.7 |

### Calculation API Example

```bash
# Login
TOKEN=$(curl -s http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin99999"}' | jq -r .token)

# Calculate life insurance net premium
curl -s -X POST http://localhost:8080/api/v1/calculate \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "formulaId": "<formula-id>",
    "inputs": {
      "sumAssured": "1000000",
      "qx": "0.001",
      "interestRate": "0.035"
    }
  }'
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

## RBAC Roles

| Permission | Admin | Editor | Reviewer | Viewer |
|-----------|-------|--------|----------|--------|
| View Formulas | Y | Y | Y | Y |
| Calculate | Y | Y | Y | Y |
| Create/Edit Formula | Y | Y | - | - |
| Delete Formula | Y | Y | - | - |
| Publish/Archive Version | Y | - | Y | - |
| Manage Tables | Y | Y | - | - |
| Manage Users | Y | - | - | - |

## Formula Model

Formulas are stored as JSON DAGs:

```json
{
  "nodes": [
    {"id": "n1", "type": "variable", "config": {"name": "age", "dataType": "integer"}},
    {"id": "n2", "type": "operator", "config": {"op": "multiply"}},
    {"id": "n3", "type": "function", "config": {"fn": "round", "args": {"places": "18"}}}
  ],
  "edges": [
    {"source": "n1", "target": "n2", "sourcePort": "out", "targetPort": "left"},
    {"source": "n2", "target": "n3", "sourcePort": "out", "targetPort": "in"}
  ],
  "outputs": ["n3"],
  "layout": {
    "positions": {"n1": {"x": 50, "y": 50}, "n2": {"x": 250, "y": 50}, "n3": {"x": 450, "y": 50}}
  }
}
```

### Node Types

| Type | Description | Config Fields |
|------|-------------|--------------|
| `variable` | Input variable | `name`, `dataType` |
| `constant` | Fixed value | `value` |
| `operator` | Arithmetic op | `op` (add/subtract/multiply/divide/power/modulo) |
| `function` | Math function | `fn` (round/floor/ceil/abs/min/max/sqrt/ln/exp), `args` |
| `subFormula` | Sub-formula ref | `formulaId`, `version` |
| `tableLookup` | Table lookup | `tableId`, `lookupKey`, `column` |
| `conditional` | If/else branch | `comparator` (eq/ne/gt/ge/lt/le) |
| `aggregate` | Aggregation | `fn` (sum/product/count/avg), `range` |

## Project Structure

```
formula-service/
├── backend/
│   ├── cmd/server/         # Entry point + seed data
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
