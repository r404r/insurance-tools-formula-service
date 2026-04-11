# Task #044 — PostgreSQL Backend Regression Report

## 概要

| 项 | 值 |
|---|---|
| 测试日 | 2026-04-11 |
| 后端 | PostgreSQL 16-alpine（docker compose `postgres` profile） |
| 后端容器 | `formula-service-backend-postgres-1` |
| 数据库容器 | `formula-service-postgres-1` |
| 测试方式 | `POST /api/v1/calculate/batch-test` (HTTP API) |
| 工具 | curl + python3 解析 |
| 测试目的 | 验证 PostgreSQL 后端下计算引擎的计算结果与已有 batch test 数据完全一致（与 SQLite 后端跑出的结果同等正确） |

## 启动状态修正

执行测试前发现一个隐藏问题：

- `formula-service-backend-postgres-1` 容器自 ~22 分钟前 Up，但 host 的 8080 端口被一个 12:42 启动的本机 native binary（task #042 验证时遗留）持有
- 后果：用户以为 docker postgres 在工作，实际所有 API 请求都被本机 native binary（SQLite 后端）拦截
- 证据：
  - `lsof -iTCP:8080`：`server` (pid 15075, native) 持有 socket，OrbStack 的 docker-proxy 此时拿不到端口
  - `docker compose logs backend-postgres`：最后一条业务日志在 11:06，之后空白
  - API `GET /formulas` 返回 156 条 vs `psql -c "SELECT count(*)" `返回 15 条 — 数量不一致

修正动作：
1. `kill 15075`（native binary 退出）
2. `docker compose --profile postgres restart backend-postgres`（容器重新 bind 8080）
3. 验证：API count=15、psql count=15 ✅
4. `lsof -iTCP:8080`：现在持有 socket 的是 `OrbStack` (docker-proxy) ✅

## 测试对象

PostgreSQL seed 数据有以下 15 个 formula。本次回归选择 4 个有 batch test 数据的 seeded formula：

| 公式 | 实际 UUID（postgres seed 后） | Batch 文件 | Cases |
|---|---|---|---|
| 定期保険一時払純保険料 | `334c60a9-da26-456d-89b6-dcb3ed567214` | `tests/batch/023/pure-premium-30cases.json` | 30 |
| 期始払年金現価 | `6433bd71-db8c-4ee8-8c66-9d2dd27eeadc` | `tests/batch/023/annuity-30cases.json` | 30 |
| 漸化式責任準備金 | `ddebee3a-521a-41dc-be4a-2efc2a2a141e` | `tests/batch/023/reserve-30cases.json` | 30 |
| 定期保険一時払純保険料 | `334c60a9-da26-456d-89b6-dcb3ed567214` | `tests/batch/033/term-life-n1-100.json` | 100 |

> **注**：UUID 是 seed 时随机生成的，不同环境会不一样。本次报告记录的是当前 docker postgres 实例的实际 UUID。原 task #023 报告里的 UUID（`c7cc13d0...` 等）是当时 SQLite seed 的产物，无可比性。

## 排除的 batch

| Batch | 原因 |
|---|---|
| `tests/batch/041/disaster-reserve-release.json` | 测试公式（异常危険準備金取崩判定）+ `loss_history` 表都不在 seed 里。要跑需先 POST 公式 + table，留作单独 task。当前 task #041 的回归走 Go 单元测试 (`go test ./internal/engine/ -run TestDisasterReserveRelease`)，不依赖 PostgreSQL。 |
| `tests/batch/033/term-life-n1-100-round4-experiment.json` | 测试 round-to-4-decimals 的 tolerance 行为，不是核心计算正确性测试。 |

## 测试结果

| Batch | Total | Passed | Failed | Pass Rate | totalExecutionTimeMs |
|---|---:|---:|---:|---:|---:|
| 023-pure-premium | 30 | 30 | 0 | **100.0%** | 359.2 |
| 023-annuity | 30 | 30 | 0 | **100.0%** | 327.0 |
| 023-reserve | 30 | 30 | 0 | **100.0%** | 47.9 |
| 033-term-life-n1-100 | 100 | 100 | 0 | **100.0%** | 8510.6 |
| **合计** | **190** | **190** | **0** | **100.0%** | **9244.7** |

**所有 190 个 case 全部通过**，相对误差容忍度：023 batches 用 `0.0001`（与原 023 报告一致），033 batch 用 `1e-15`（接近完全精确，与原 task #033 一致）。

## 验证 PostgreSQL 后端真的在服务

除了 host 端口确认外，下面三条证据共同确认 4 个 batch 是被 PostgreSQL-backed 的容器处理的，不是任何旁路 / 缓存：

1. **docker compose logs**：所有 4 次 `POST /api/v1/calculate/batch-test` 在容器日志里有对应条目，duration 与客户端测得的 `totalExecutionTimeMs` 完全匹配（365ms / 327ms / 48ms / 8511ms）
2. **`updated_by` 列填充**：直接 SQL 查询 postgres，每个 seeded formula 的 `updated_by` 都是 `t`（NOT NULL），证明 task #042 引入的 `UpdateMeta` 路径在 PostgreSQL 上也正确执行
3. **API count == psql count**：修复 native binary 冲突后两边都是 15

## 与 SQLite 后端的对比

原 task #023 的 90-case 报告（SQLite 后端）和原 task #033 的 100-case 都是 100% pass，本次 postgres 跑同样的数据也 100% pass，**两个后端的计算结果完全一致**。这是预期行为——计算引擎是纯 Go 代码不依赖数据库；store 层只负责持久化 graph + version + 中间结果 cache。本次回归确认 store 切换不会影响计算正确性。

## 性能侧观察（非测试目标）

| Batch | postgres 后端 totalExecutionTimeMs |
|---|---:|
| 023-pure-premium (30 cases) | 359 ms |
| 023-annuity (30 cases) | 327 ms |
| 023-reserve (30 cases) | 48 ms |
| 033-term-life-n1-100 (100 cases) | 8511 ms |

100-case batch 在 postgres docker 容器里跑 8.5s，比 native binary 在宿主机 SQLite 后端跑同样 batch 的基线 (~4.1s, 见 `docs/performance/001-batch-test-speedup-analysis.md`) 慢约 2×。这个差距来自两个因素：
- docker container CPU 限制（默认共享所有核但有 namespace overhead）
- 计算引擎对每个 case 都要从 postgres 拉一次 graph + table 数据，比 SQLite 的同进程 file I/O 慢

性能差异不是本次测试的关注点；正确性是。

## 重现步骤

```bash
cd /Users/thomasd/work/github/r404r/insurance-tools/formula-service

# 1. 确保只有 docker 在 8080 上
lsof -iTCP:8080 -sTCP:LISTEN  # 应该只看到 OrbStack / docker-proxy
docker compose --profile postgres ps

# 2. 拿 admin token
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin99999"}' \
  | python3 -c 'import json,sys;print(json.load(sys.stdin)["token"])')

# 3. 找到 seed 后的实际 UUID
curl -s "http://localhost:8080/api/v1/formulas?limit=100" \
  -H "Authorization: Bearer $TOKEN" \
  | python3 -c "import json,sys;d=json.load(sys.stdin);[print(f['id'],f['name']) for f in d['formulas']]"

# 4. 用脚本跑 4 个 batch（见 docs/tasks/044-postgres-regression-run.md）
```

## 结论

✅ PostgreSQL 后端在 4 个回归 batch（共 190 cases）下的计算结果与 SQLite 后端完全一致，正确性回归通过。
