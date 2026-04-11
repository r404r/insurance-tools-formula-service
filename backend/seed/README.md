# Seed bundles

This directory holds the canonical built-in seed data for formula-service:

- `tables/*.json` — lookup tables (one `CreateTableRequest` per file)
- `formulas/*.json` — formulas (one `ExportBundle` per file)

The `seed-runner` CLI in `runner/` reads these files at startup time and
loads them into a running backend through the public HTTP API. There are
no special "seed-only" endpoints — `seed-runner` uses the same routes a
human admin would use.

Before task #047, this same data was hard-coded in
`backend/cmd/server/main.go` (~1200 lines of `domain.FormulaGraph{}` literals).
The hard-coded form made small fixes expensive (rebuild + restart per change),
made graph correctness fragile (the bundled validator was bypassed), and made
`main.go` too big to review. Moving to JSON bundles + an out-of-process loader
fixes all three.

## File layout

```
backend/seed/
├── README.md              ← you are here
├── embed.go               ← go:embed directive (used by reset handler)
├── embed_test.go
├── formulas/              ← 31 formula bundles (one per file)
│   ├── 010-寿险净保费计算.json
│   ├── 020-…
│   └── 310-日本自賠責-収支調整剰余金.json
├── tables/                ← 2 lookup tables
│   ├── 010-日本標準生命表2007-簡易版.json
│   └── 020-claims_triangle_sample.json
└── runner/                ← seed-runner CLI (package main)
    ├── main.go
    └── main_test.go
```

The numeric prefix on each filename is the **dependency order**: lex sort
must place every dependency before its consumer. `seed-runner` processes
files in lex order, so as long as a sub-formula body or referenced table
sorts ahead of a formula that uses it, placeholder substitution succeeds.
The script that generated the initial set used multiples of 10 (`010`,
`020`, …) to leave room for inserts.

## Bundle format

Tables (`tables/*.json`):

```json
{
  "name": "claims_triangle_sample",
  "domain": "property",
  "tableType": "loss_triangle",
  "data": [ { "acc_year": "2020", "dev_year": "1", ... }, ... ]
}
```

Formulas (`formulas/*.json`) — same shape as the existing import/export
bundle (see `internal/api/formula_handler.go`):

```json
{
  "version": "1.0",
  "exportedAt": "2026-04-11T14:18:33Z",
  "formulas": [
    {
      "sourceId": "日本損害保険 チェインラダー LDF",
      "sourceVersion": 1,
      "name": "日本損害保険 チェインラダー LDF",
      "domain": "property",
      "description": "...",
      "graph": { "nodes": [...], "edges": [...], "outputs": [...] }
    }
  ]
}
```

Each bundle file contains exactly one formula. Multi-formula bundles will
be rejected by `peekFormulaName` in the runner.

## Cross-references: placeholder syntax

The graph of one seed formula often references another seed object by ID:

- `loop` nodes have a `formulaId` pointing at the body sub-formula
- `tableLookup` and `tableAggregate` nodes have a `tableId`

Bundles must use a **placeholder** instead of a real UUID:

```json
{
  "id": "loop_px",
  "type": "loop",
  "config": {
    "formulaId": "{{formula:生存率因子 1-qx}}",
    "iterator": "k",
    "aggregation": "product"
  }
}
```

Placeholder grammar:

| Token | Resolved at run time to |
|---|---|
| `{{formula:NAME}}` | The real UUID of a formula whose `name` matches `NAME` |
| `{{table:NAME}}` | The real UUID of a table whose `name` matches `NAME` |

`seed-runner` substitutes placeholders just before each `POST /import`
call, using a name → id map built from objects already created in this
run plus any objects already in the database.

**Don't put `{{formula:…}}` or `{{table:…}}` inside a description string**
unless you actually want it substituted — the rewriter operates on the raw
bundle bytes and doesn't distinguish JSON value positions from comment-like
content.

## Running seed-runner

```bash
# Build the runner once (from the backend module dir, where go.mod lives)
cd backend && go build -o /tmp/seed-runner ./seed/runner

# Run from the project root so the default --seed-dir path resolves
cd ..
/tmp/seed-runner --seed-dir backend/seed

# Or via env vars (handy for docker / CI). When the runner runs inside a
# container, --seed-dir / SEED_DIR should point at the bundle directory
# inside the container, not the host path.
SEED_BASE_URL=http://backend:8080 \
SEED_ADMIN_USER=admin \
SEED_ADMIN_PASS=admin99999 \
/tmp/seed-runner --seed-dir /seed
```

Flags:

| Flag | Default | Env var |
|---|---|---|
| `--base-url` | `http://localhost:8080` | `SEED_BASE_URL` |
| `--admin-user` | `admin` | `SEED_ADMIN_USER` |
| `--admin-pass` | `admin99999` | `SEED_ADMIN_PASS` |
| `--seed-dir` | `backend/seed` | `SEED_DIR` |

The runner is **idempotent in single-writer use**: it queries the backend
for existing names before creating anything, so re-running against a
populated DB is a no-op that prints `skip` lines and exits 0.

> ⚠️ The list-then-create check is **TOCTOU**, not transactional. If a
> separate process creates a formula or table with the same name between
> the runner's `GET /api/v1/formulas?…` and its `POST /import`, the runner
> will create a duplicate. Backend storage has no name uniqueness
> constraint and the formula list page will show both copies. Don't run
> the seed runner concurrently with normal user traffic; reserve it for
> deployment-time bootstrap.

The runner uses login as a readiness probe and retries with a 60-second
budget, so it's safe to start it concurrently with the backend container —
it will block until the backend (and its DB) are accepting requests.

## Reset workflow

`POST /api/v1/admin/reset-seed` (admin only, JWT bearer required) deletes every formula and
table whose name matches an embedded seed bundle. It does **not** re-create
them — to re-populate, run `seed-runner` afterwards. The handler used to
inline its own re-seed logic; that path was removed when the seed code
moved out of `main.go`.

The "which names are seed-owned" list is read from a `go:embed` snapshot
of this directory taken at backend build time (see `embed.go`). If you
add a new seed bundle, you must rebuild the backend binary for the reset
handler to pick it up; the runner itself reads from disk and doesn't need
a rebuild.

## Adding a new seed formula

1. Build it interactively in the visual editor and save it.
2. Export it: `POST /api/v1/formulas/export` with the formula's ID, save
   the response body as `backend/seed/formulas/NNN-slug.json`. Pick a
   numeric prefix that lex-sorts after every formula and table this one
   depends on.
3. If the new formula references another seed formula or table by UUID,
   replace each UUID with the matching `{{formula:NAME}}` or `{{table:NAME}}`
   placeholder.
4. (Optional) Pretty-print: `python3 -m json.tool < new.json > pretty.json`.
5. Run the runner against a clean DB (`POST /api/v1/admin/reset-seed` then
   `seed-runner`) to verify nothing breaks.
6. Rebuild the backend binary so the reset handler tracks the new name.
7. Add the formula's expected calculation result to the regression suite
   if it's a load-bearing actuarial formula.

## Adding a new lookup table

1. Build the table in the lookup-tables UI or via `POST /api/v1/tables`.
2. `GET /api/v1/tables/{id}` to fetch the full payload.
3. Save the relevant fields (name, domain, tableType, data) into
   `backend/seed/tables/NNN-slug.json` matching the schema above.
4. Same rebuild + reset note as for formulas.

## Known limitations (stage 1)

- **No `--dry-run`**: stage 2 will add a flag to parse and validate
  bundles without making API calls.
- **No `--only NAME`**: stage 2 will add single-formula seeding for
  faster iteration.
- **No multi-formula bundles**: each file holds exactly one formula. The
  format supports more, but the runner enforces single-entry on read.
- **Sub-domain organization**: bundles live in a flat `formulas/`
  directory with numeric prefixes. Stage 2 will optionally split into
  `formulas/{life,property,auto}/` subdirectories.
- **Reset doesn't re-seed**: see above.
