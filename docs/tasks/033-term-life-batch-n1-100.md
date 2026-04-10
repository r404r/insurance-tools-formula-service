# Task #033: 定期保険一時払純保険料 批量测试数据 (n=1..100)

## Status: done

## 需求

为公式 `ca2d07df-0768-4eee-98c8-1eac67f7d741`（定期保険一時払純保険料，
Term life single pure premium: `S * Σ_{t=1}^{n} v^t * _{t-1}p_x * q_{x+t-1}`）
生成批量测试数据，n 从 1 到 100，共 100 条 test case，
可直接导入到 Batch Test 画面执行回归测试。

## 设计

### 约束发现

公式内部通过 loop 节点调用子公式 `ed23e622-...`（死亡給付PV項），
子公式查询 qx 死亡率表。表的年龄范围 **0–100 岁**，
第 t 次迭代使用 `q_{x+t-1}`，因此 `x + n − 1 ≤ 100`。

### 固定参数（用户确认方案 A）

| 参数 | 值 | 说明 |
|---|---|---|
| S | `10000000` | 保険金額 1000 万円 |
| x | `1` | 年齢 1 歳（使 n 可覆盖 1–100） |
| v | `0.970873786407766990` | 割引因子 ≈ 1/1.03（3% 利率） |
| n | 1..100 | 唯一变量 |

### 生成方式

1. 写一个一次性 Python 脚本，调用 `/api/v1/calculate` 100 次，
   每次只变动 `n`
2. 读取响应的 `result.op_result`，作为 `expected.op_result`
3. 输出到 `tests/batch/033/term-life-n1-100.json`，
   遵循既有 `tests/batch/023/` 的 `[{label, inputs, expected}, ...]` 格式
4. `label` 格式：`case-NNN-n{n}`（例如 `case-001-n1`, `case-100-n100`）

### Expected 值的语义

用引擎输出作为 expected 值 = **回归基线**：
锁定当前引擎行为。后续如果引擎/qx 表/子公式变更导致结果漂移，
这个 batch test 可以捕获。

## 涉及文件

- `tests/batch/033/term-life-n1-100.json`（新建）
- `docs/tasks/033-term-life-batch-n1-100.md`（本文件）

## TODO

- [x] 调研公式结构和 qx 表约束
- [x] 与用户确认参数（方案 A：S=10M, x=1, v≈1/1.03）
- [x] 写生成脚本并执行
- [x] 验证输出 JSON 格式正确、条数为 100
- [x] 抽样核对几个值与直接调用 API 一致
- [x] 整体灌入 `/calculate/batch-test` 端点：100/100 通过 ✓
- [x] commit（纯数据变更）

## 完成标准

- [x] `tests/batch/033/term-life-n1-100.json` 包含恰好 100 条 case
- [x] 每条 case 结构正确：`{label, inputs: {S,x,n,v}, expected: {op_result}}`
- [x] 首条 n=1、末条 n=100
- [x] 单调性检查：expected.op_result 随 n 严格递增（term 公式性质）
- [x] 全量跑通 batch-test 端点：**100/100 pass, rate=100**

## 关键验证数据

| n | op_result |
|---|---|
| 1 | 3883.49514563106796 |
| 50 | 145054.526474930235077785 |
| 100 | 1117976.009218054499974591 |
