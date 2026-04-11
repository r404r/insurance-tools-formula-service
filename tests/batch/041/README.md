# Batch 041 — Disaster Reserve Release (integration regression for tasks #039 + #040)

## 用途

这是一组真实业务场景测试数据，用于回归验证：

- **Task #039** — Composite Conditional 节点（AND/OR/NOT 多条件）
- **Task #040** — TableAggregate 节点（带 filter 的表聚合）

灵感来自日本损害保险数理 spec（[`docs/specs/002-Japan-insurance-ref.md`](../../../docs/specs/002-Japan-insurance-ref.md) §19 異常危険準備金）。

## 测试公式语义

```
IF avg_historical_loss_ratio > 0.5
   AND current_loss_ratio    > 0.5
   AND NOT (is_safe_zone == 1)
THEN release release_amount
ELSE 0
```

- `avg_historical_loss_ratio` 由 **TableAggregate** 计算：从 `loss_history` 表中
  对所有 `year < current_year` 的行的 `loss_ratio` 列取平均
- 三个条件用 **Composite Conditional**（combinator=and）合取，第三个用 `Negate=true`
  实现 NOT 语义

### 参考表 `loss_history`

| year | loss_ratio |
|---:|---:|
| 2018 | 0.40 |
| 2019 | 0.60 |
| 2020 | 0.80 |
| 2021 | 0.40 |
| 2022 | 0.20 |

每个 `current_year` 对应的历史平均（用于复算 expected）：

| current_year | 包含年份 | avg_historical |
|---:|---|---:|
| 2019 | 2018 | 0.40 |
| 2020 | 2018-2019 | 0.50 |
| 2021 | 2018-2020 | 0.60 |
| 2022 | 2018-2021 | 0.55 |
| 2023 | 2018-2022 | 0.48 |

## 文件清单

- `disaster-reserve-release.json` — 20 组 cases（label / inputs / expected）

## 如何运行

### 方式 1：Go 单元测试（首选，全自动）

`backend/internal/engine/integration_039_040_test.go` 已经包含同样的 20 cases，
随 `go test ./...` 自动跑。最快验证：

```bash
cd backend && go test ./internal/engine/ -run TestDisasterReserveRelease -race -v
```

### 方式 2：通过 API 跑 batch test

如果想从 HTTP 端到端验证（比如调试 API 路由或前端），需要：

1. 把 `disaster-reserve-release.json` 中的 20 cases 通过前端的 Batch Test 页面上传
2. 选择对应的公式 ID（需要先在系统里创建一个等价图，因为复合 Conditional + TableAggregate 目前只能通过 API/手写 JSON 创建）
3. 运行并比对 expected 与 actual

> **注意**：当前 visual editor 还没有 composite Conditional 和 TableAggregate 的 UI（见 `docs/backlog.md` 未来研究项目），所以 API 路径需要手工 POST graph JSON。

## 测试覆盖范围

20 组 cases 分布如下：

| 验证目标 | cases |
|---|---|
| 全 true → 进入 then 分支 | 02, 03, 08, 12, 13, 16, 19 |
| condition_0 false（avg 不够高） | 01, 04, 05, 14, 17, 18 |
| condition_1 false（curr 不够高） | 06, 07 |
| condition_2 false（safe zone） | 09, 10, 15 |
| 多 false 组合 | 20 |
| 边界值（gt 不取等） | 04, 07 |
| Release 金额变化 | 11（=0）, 12（5000）, 16（12345）, 19（7777） |
| 历史平均跨不同 current_year | 全部覆盖 2019-2023 |

## 维护

如果未来修改 `loss_history` 表的内容或 condition 阈值，需要同步更新：

1. `disaster-reserve-release.json` 的 expected 值
2. `backend/internal/engine/integration_039_040_test.go` 的 case 表
3. `docs/tasks/041-integration-test-039-040.md` 的手算预期表
