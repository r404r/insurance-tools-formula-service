# Task #041: 集成回归测试 — 复合 Conditional + TableAggregate

## Status: done

## 需求

针对 task #039（Conditional AND/OR/NOT）和 task #040（NodeTableAggregate）这两次引擎扩展，设计一个**真实业务场景**的测试公式 + 20 组测试数据 + 期望结果，并把它加入到回归测试套件中。

## 设计

### 测试公式：异常危险准备金取崩判定

灵感来自 spec 002 公式 19「異常危険準備金の積立と取崩し」。规则是：

> **当满足以下三条全部时**（AND），按 `release_amount` 全额取崩；否则取崩 0：
> 1. 历史平均损失率 > 0.5（即近年风险确实偏高）
> 2. 当年实际损失率 > 0.5（即今年也确实超出预期）
> 3. **不在**安全区域（is_safe_zone == 1 时不取崩）— 用 `Negate: true` 实现 NOT

这个公式同时把两个新功能用上：
- **TableAggregate** 计算「current_year 之前所有年份的平均损失率」
- **Composite Conditional** 用 3-term AND + Negate 表达三条业务规则的合取

### 公式图结构

```
loss_history 表（输入数据）:
  | year | loss_ratio |
  | 2018 |    0.40    |
  | 2019 |    0.60    |
  | 2020 |    0.80    |
  | 2021 |    0.40    |
  | 2022 |    0.20    |

节点:
  ── 变量 ──
  v_current_year         (Variable: current_year)
  v_current_loss_ratio   (Variable: current_loss_ratio)
  v_is_safe_zone         (Variable: is_safe_zone)
  v_release_amount       (Variable: release_amount)

  ── 常量 ──
  c_avg_threshold        (Constant: 0.5)
  c_curr_threshold       (Constant: 0.5)
  c_safe_marker          (Constant: 1)
  c_zero                 (Constant: 0)

  ── TableAggregate ──
  agg (TableAggregate):
    tableId:     loss_history
    aggregate:   avg
    expression:  loss_ratio
    filters:     [{column: year, op: lt, inputPort: bound}]
    inputs:      bound ← v_current_year

  ── Composite Conditional ──
  cond (Conditional, composite mode):
    combinator:  and
    conditions:
      [0]: { op: gt }                       # avg_historical > 0.5
      [1]: { op: gt }                       # current_loss_ratio > 0.5
      [2]: { op: eq, negate: true }         # NOT (is_safe_zone == 1)
    inputs:
      condition_0       ← agg               # 历史平均
      conditionRight_0  ← c_avg_threshold
      condition_1       ← v_current_loss_ratio
      conditionRight_1  ← c_curr_threshold
      condition_2       ← v_is_safe_zone
      conditionRight_2  ← c_safe_marker
      thenValue         ← v_release_amount
      elseValue         ← c_zero

输出: cond
```

### 历史平均损失率（手算预期）

| current_year | 包含年份 | avg_loss_ratio |
|---:|---|---:|
| 2019 | 2018 | 0.40 |
| 2020 | 2018-2019 | 0.50 |
| 2021 | 2018-2020 | 0.60 |
| 2022 | 2018-2021 | 0.55 |
| 2023 | 2018-2022 | 0.48 |

全部精确（无循环小数），方便严格对比。

### 20 组测试数据

每组覆盖一种典型业务场景。`c0/c1/c2` 列展示三个 condition 的结果，`result` 列是手算的期待值。

| # | current_year | curr_lr | safe | release | avg_hist | c0 (>0.5) | c1 (>0.5) | c2 (NOT 1) | result | 测试目的 |
|---|---:|---:|:---:|---:|---:|:---:|:---:|:---:|---:|---|
| 01 | 2023 | 0.70 | 0 | 100 | 0.48 | F | T | T | 0 | c0 false → AND fail |
| 02 | 2022 | 0.70 | 0 | 100 | 0.55 | T | T | T | 100 | 全 true → release |
| 03 | 2021 | 0.70 | 0 | 100 | 0.60 | T | T | T | 100 | 全 true，不同年份 |
| 04 | 2020 | 0.70 | 0 | 100 | 0.50 | F | T | T | 0 | avg=阈值，gt 不取等 |
| 05 | 2019 | 0.70 | 0 | 100 | 0.40 | F | T | T | 0 | 最早年份的边界 |
| 06 | 2022 | 0.40 | 0 | 100 | 0.55 | T | F | T | 0 | c1 false |
| 07 | 2022 | 0.50 | 0 | 100 | 0.55 | T | F | T | 0 | curr=阈值，gt 不取等 |
| 08 | 2022 | 0.51 | 0 | 100 | 0.55 | T | T | T | 100 | 阈值刚刚好之上 |
| 09 | 2022 | 0.70 | 1 | 100 | 0.55 | T | T | F | 0 | safe zone → c2 false |
| 10 | 2021 | 0.70 | 1 | 100 | 0.60 | T | T | F | 0 | safe zone（不同年份） |
| 11 | 2022 | 0.70 | 0 | 0 | 0.55 | T | T | T | 0 | release=0 也走 then |
| 12 | 2022 | 0.70 | 0 | 5000 | 0.55 | T | T | T | 5000 | 大额 release |
| 13 | 2021 | 0.60 | 0 | 250 | 0.60 | T | T | T | 250 | 普通 release |
| 14 | 2020 | 0.60 | 0 | 250 | 0.50 | F | T | T | 0 | c0 false 不同年 |
| 15 | 2021 | 0.51 | 1 | 999 | 0.60 | T | T | F | 0 | c1+c2 边界 |
| 16 | 2022 | 0.61 | 0 | 12345 | 0.55 | T | T | T | 12345 | 不规则 release |
| 17 | 2023 | 0.99 | 0 | 100 | 0.48 | F | T | T | 0 | c0 false 极端 curr |
| 18 | 2019 | 0.99 | 0 | 100 | 0.40 | F | T | T | 0 | 最低 avg |
| 19 | 2022 | 0.55 | 0 | 7777 | 0.55 | T | T | T | 7777 | curr 略高于阈值 |
| 20 | 2022 | 0.50 | 1 | 100 | 0.55 | T | F | F | 0 | c1+c2 同时 false |

### 实现位置

- `backend/internal/engine/integration_039_040_test.go`（新建）—— Go 单元测试，自动随 `go test ./...` 跑
- `tests/batch/041/disaster-reserve-release.json`（新建）—— 同样的 20 cases 以批量测试 JSON 格式留存，便于通过 API 复跑或人工检验

## 涉及文件

- `backend/internal/engine/integration_039_040_test.go`（新建）
- `tests/batch/041/disaster-reserve-release.json`（新建）
- `tests/batch/041/README.md`（新建，描述 batch 用途和如何重跑）

## TODO

- [x] 创建 Go 集成测试，构建 graph + 20 cases + 断言
- [x] 创建 batch JSON 数据文件
- [x] 创建 batch 目录 README
- [x] `go test ./internal/engine/ -run TestDisasterReserveRelease -race -v` 全绿
- [x] codex review → LGTM（无 finding）→ commit

## 完成标准

- [x] 20 组 cases 的 expected 与引擎实际输出一致
- [x] 测试同时验证 task #039 和 #040 的代码路径
- [x] 测试加入到 `go test ./...` 默认跑的回归集
- [x] race detector 通过
