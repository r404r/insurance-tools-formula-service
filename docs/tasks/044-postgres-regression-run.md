# Task #044: PostgreSQL 后端正确性回归测试

## Status: done

## 需求

用户启动了 docker compose 的 postgres profile，想验证用 PostgreSQL 作为后端时计算引擎的正确性。基于已有的 batch test 数据跑一轮回归测试，确认 SQLite 和 PostgreSQL 的计算结果一致。

## 现状诊断

启动状态有冲突：
- `formula-service-backend-postgres-1` 容器 22 分钟前启动，docker ps 显示 Up
- `pid 15075`（我之前为 task #042 sort 验证启动的 native binary）从 12:42 开始就 holding 8080 端口
- `docker compose logs backend-postgres` 最后一条日志在 11:06 — 12:42 之后所有 API 请求都被 native binary 拦截，根本没到 postgres 容器
- 直接 SQL 查询 postgres 容器：15 个 formula；API 返回：156 个（来自 native binary 的 SQLite）

**结论**：用户实际上一直在测 SQLite，没真测过 PostgreSQL。

## 设计

### 第 1 步：让 docker 容器真的能服务流量

1. 停掉 native binary 15075
2. Restart `backend-postgres` 容器让它重新 bind 8080
3. 直接 SQL count + API count 比对，确认一致

### 第 2 步：选可复用的 batch test case

| Batch | 文件 | Cases | 适用性 |
|---|---|---|---|
| 023 纯保險料 | `tests/batch/023/pure-premium-30cases.json` | 30 | ✅ 目标公式 `定期保険一時払純保険料` 已 seed |
| 023 年金現価 | `tests/batch/023/annuity-30cases.json` | 30 | ✅ 目标公式 `期始払年金現価` 已 seed |
| 023 责任準備金 | `tests/batch/023/reserve-30cases.json` | 30 | ✅ 目标公式 `漸化式責任準備金` 已 seed |
| 033 100-case n=1..100 | `tests/batch/033/term-life-n1-100.json` | 100 | ✅ 同一 `定期保険一時払純保険料`，但 n=1..100 |
| 041 disaster reserve | `tests/batch/041/disaster-reserve-release.json` | 20 | ❌ 公式 + `loss_history` 表都不在 seed 里，需要先 POST 公式+表才能跑，留作后续 |

### 第 3 步：执行 + 比对

每个 batch 通过 `POST /api/v1/calculate/batch-test` 跑，handler 内部对每个 case 计算并和 expected 比对。回归 PASS = 所有 case 全部 pass。

### 第 4 步：报告

把汇总结果落盘到 `tests/reports/044-postgres-regression.md`，作为永久记录。

## 涉及文件

- `tests/reports/044-postgres-regression.md`（新建）
- 不修改任何代码

## TODO

- [x] 停 native binary 15075
- [x] Restart docker backend-postgres
- [x] 确认 API count == postgres count（15 == 15）
- [x] 查 seed 后的实际 formula UUID（4 个目标公式）
- [x] 跑 4 个 batch（023×3 + 033×1，共 190 cases）
- [x] 生成 `tests/reports/044-postgres-regression.md` 报告
- [x] commit

## 结果

| Batch | Cases | Pass | totalExecutionTimeMs |
|---|---:|---:|---:|
| 023-pure-premium | 30 | 30 | 359 |
| 023-annuity | 30 | 30 | 327 |
| 023-reserve | 30 | 30 | 48 |
| 033-term-life-n1-100 | 100 | 100 | 8511 |
| **合计** | **190** | **190** | **9245** |

**100% 通过**。详见 [`tests/reports/044-postgres-regression.md`](../../tests/reports/044-postgres-regression.md)。

## 完成标准

- [x] 4 个 batch 全部 100% pass
- [x] 报告记录每个 batch 的耗时 + 通过率
- [x] 用户可以在 PostgreSQL 后端下重复同样的回归
