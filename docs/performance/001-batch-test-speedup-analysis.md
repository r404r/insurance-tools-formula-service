# Batch Test Speedup 分析：平台原因与优化方向

**日期**：2026-04-11
**背景**：Task #036 实现了 BatchTest 服务端并行化后，在 task #033 的 100 case
（定期保険一時払純保険料，带 loop + table 的公式）上实测只获得约 2.5× 加速，
远低于 worker 数理论上限。本报告分析平台原因并提出后续优化方向。
**测试环境**：Apple Silicon M 系列，`sysctl hw.ncpu` = 8（8 物理核心）。

---

## 1. 观察到的平台现象

对 task #033 数据跑不同 worker 数（通过 admin settings 动态调整 `maxConcurrentCalcs`
触发不同 `floor(limit/5)`）：

| maxConcurrentCalcs | workers | 冷缓存耗时 |
|---:|---:|---:|
| 5   | 1   | 10,338 ms |
| 10  | 2   | ~6,200 ms |
| 20  | 4   | ~4,400 ms |
| 40  | 8   | 4,010 ms |
| 100 | 20  | 4,006 ms |
| 500 | 100 | 4,006 ms |

**结论**：8 worker 之后完全平台，曲线不再下降。

---

## 2. 平台的真正原因（不是锁竞争）

### 2.1 effective parallelism 测量

引入 `effective_parallelism = sum(case.executionTimeMs) / totalExecutionTimeMs`，
表示"平均同时有多少个 case 在真正跑"。这个比值揭示的是实际的并行效率，
而不只是墙上时间。

| mode | wall | per-case 总和 | effective | per-case 平均 |
|---|---:|---:|---:|---:|
| workers=1   | 10,267 ms | 10,266 ms | **1.00×** | 103 ms |
| workers=2   |  6,205 ms | 12,384 ms | **2.00×** | 124 ms |
| workers=4   |  4,388 ms | 17,321 ms | **3.95×** | 173 ms |
| workers=8   |  4,007 ms | 31,523 ms | **7.87×** | 315 ms |
| workers=20  |  4,061 ms | 32,220 ms | **7.93×** | 322 ms |
| workers=100 |  4,087 ms | 32,151 ms | **7.87×** | 322 ms |

### 2.2 两个关键发现

1. **Parallelism 完美线性扩展到 8 worker**（effective = 7.87×，接近理论上限 8）。
   worker 之间没有明显相互阻塞——最初怀疑的"engine cache/table resolver 锁争抢"
   不是瓶颈，否则 effective 数会在 4 或 8 之前就提前封顶。

2. **每个 case 的平均耗时随并发度上涨**：
   - 单线程：103 ms/case
   - 8 并发：315 ms/case（**慢 3×**）

   这不是等锁，是每个 case 自己变慢了。

### 2.3 结论

**平台是硬件并行度上限，不是软件锁竞争**：

```
workers=8 时：
  - effective parallelism ≈ 8
  - 同时在跑 ≈ 8 个 case
  - 刚好把 8 个物理核心打满
  - 每个 case 在自己那块 CPU 上"全速"运行
  - 但所有核同时满载时，共享资源（L3 cache、memory bandwidth、
    branch predictor、Go scheduler）被 8 个 goroutine 瓜分
  - 每个 case 的单核吞吐从 103ms 降到 315ms（3× 慢）
  - 总吞吐仍然是 8 / 315ms ≈ 25 cases/sec，对应 4s 完成 100 cases ✓
```

> 这同时也解释了为什么纯算术公式（task #036 测试里的 `财产险保费计算`，
> 无 loop / 无 table）在 100 cases 只要 6ms → 2ms 就到顶了：
> 两者都是 CPU-bound，只是绝对计算量不同。

---

## 3. 方向研究：如何真正拿到更多 speedup

既然硬件并行度封顶，想获得更大 speedup 必须从**减少总计算量**入手，而不是
让 CPU 跑更快。

### 方向 A：跨 case 的 loop 迭代结果 memoization ⭐⭐⭐ 最大收益

#### 3.1 当前行为

`backend/internal/engine/engine.go:executeLoop` 第 400–438 行：

```go
for t := start; t <= end; t += step {
    childInputs := cloneDecimalMap(baseChildInputs)
    childInputs[cfg.Iterator] = current
    allResults, err := e.executor.Execute(loopCtx, bodyPlan, childInputs)
    // ...
}
```

每个 loop 迭代**直接调用** `executor.Execute`，绕过了 `engine.cache` 和完整
`Calculate` 路径。

#### 3.2 为什么这是浪费

task #033 的 100 个 case：
- 固定参数：S=10,000,000 / x=1 / v=0.97087... （所有 case 相同）
- 变量：n = 1, 2, ..., 100

对于 case n=50 和 case n=51，loop body `ed23e622`（死亡給付PV項）的迭代
t=1..50 的计算结果**字节完全一致**——都是 `死亡給付PV項(t, x=1, v=0.97...)`。

当前白做的工：
- 100 个 case 共需 Σ(n=1..100) = **5050** 次 body 迭代
- 实际独立的 (t, x, v) 组合只有 **100** 个（t=1..100）
- 浪费 = 5050 − 100 = **4950 次重复**（98% 的 body 执行都是重复劳动）

#### 3.3 修复方案

把 loop body 的每次迭代**走完整 Calculate 路径**而不是直接 `executor.Execute`。
这样 `engine.cache` 就能跨迭代、跨 case 命中。

```go
// 当前：
allResults, err := e.executor.Execute(loopCtx, bodyPlan, childInputs)

// 改为（伪代码）：
result, err := e.Calculate(loopCtx, &version.Graph, stringifyInputs(childInputs))
// Calculate 自己会先查 cache
```

需要仔细处理的点：
- `CacheKey.InputHash` 必须能区分不同 (t, x, v, ...) 组合（当前 `ComputeInputHash`
  看起来可以，需要验证）
- `bodyPlan` 的重用 vs 每次走 Calculate 会重建 plan——需要做一个"已有 plan 直接
  execute + 包上 cache"的辅助路径
- Fold loop 不能走这条路径（有状态依赖）——只对 sum/product/count/avg/min/max
  这类独立迭代生效

#### 3.4 预期收益

- 理论：100 个 case 走完后，共 100 次真实 body 执行 + 4950 次 cache hit
- Cache hit 是 μs 级（只涉及 map 查找 + 深拷贝）
- 单线程就能把 10 秒的 batch 压到 **< 500 ms**
- Speedup 估计：**10–20×**（相对当前并行 8 worker 方案）

#### 3.5 风险

- **Cache key 正确性**——必须严格保证相同输入总产生相同 key
- **Cache 内存占用上升**——一个 loop 可能产生数百个 cache entry，默认
  cacheSize=1000 可能不够，需要加大或做独立的 sub-formula cache
- **对非重复输入的 workload 无收益**——但也不会更慢
- **Fold 模式不能优化**——状态依赖

---

### 方向 B：table 数据缓存 ⭐⭐ 低成本次要收益 ✅ 已实现（task #037）

#### 3.6 当前行为

`backend/internal/engine/engine.go:preloadTableData`（第 686 行）+
`backend/internal/engine/table_resolver.go:ResolveTable`：

每次 `Calculate` → 每次 `executeLoop` → 一次 `preloadTableData` → 一次
`tableResolver.ResolveTable`，每次都做：

1. SQLite 查询 `tables` 表（全量 JSON 数据）
2. `json.Unmarshal` 整个数组
3. 对每 row 调 `decimal.NewFromString` parse
4. 建 `map[compositeKey]Decimal`

对 task #033 的 100 cases，这套动作重复 **100 次**，每次约 1–3 ms，
**合计 100–300 ms 被用在重复 parse 同一张 qx 表上**。

#### 3.7 修复方案

在 `StoreTableResolver` 上加一个基于 `(tableID, keyColumns hash, column)`
的缓存。命中时直接返回缓存的 map（深拷贝或只读共享）。失效条件：
`table.UpdatedAt` 变化。

```go
type cachedResolvedTable struct {
    data      map[string]string
    updatedAt time.Time
}

type CachedTableResolver struct {
    inner store.TableRepository
    mu    sync.RWMutex
    cache map[string]cachedResolvedTable
}
```

#### 3.8 预期收益

- 单次 batch 节省 100–300 ms（相对 4000 ms 总耗时约 **3–7%**）
- 对 Postgres/MySQL 后端收益更大——减少 DB round-trip
- 改动集中在 `table_resolver.go`，低风险

---

### 方向 C：减少 shopspring/decimal 分配压力 ⭐ 不推荐

`shopspring/decimal` 所有运算都分配新 Decimal（immutable by design），对于
5050 次 body 迭代 × ~20 次 decimal 运算 = **约 10 万次堆分配**。这对 Go GC
有明显压力。

可能的修复：
1. 改用 `cockroachdb/apd`（原地修改的高精度 decimal）——**改动非常大**
2. 用 `sync.Pool` 池化中间 Decimal——**shopspring 内部用 big.Int，pool 效果有限**
3. 降低精度（`intermediatePrecision` 28 → 20）——**用户已明确要高精度**

方向 C 收益很小（~1.1×）且工程成本高，**不建议单独做**。如果将来整个 decimal
栈想换库可以顺带评估。

---

### 方向 D：禁用 engine 内部 DAG parallelism 在 batch 上下文中 ⭐⭐

#### 3.9 当前行为

`backend/internal/engine/parallel.go` 第 91 行：

```go
if len(level) >= parallelThreshold && ex.workers > 1 {
    if err := ex.executeParallel(ctx, plan, level, results, inputs); err != nil {
        return nil, err
    }
}
```

engine 内部的 `executor.workers` 默认 > 1，所以每个 `Calculate` 在遇到含 ≥4
节点的 level 时会 spawn 自己的 goroutines。当 batch 的 8 个 worker 每个再
spawn 4 个内部 goroutine，goroutine 总数 = **32**，在 **8 核上过度订阅**。

#### 3.10 修复方案

batch test 路径下，把 engine 内部 workers 强制为 1，让外层 batch worker 独占
并行度。两种做法：

1. 给 `executor.Execute` 加一个可选的 `ExecuteSequential` 入口
2. 让 Engine 提供 `CalculateSequential(ctx, graph, inputs)` 入口，batch 用这个

#### 3.11 预期收益

- 消除 over-subscription
- 单 case 耗时可能从 315 ms 回落到 ~200 ms
- Speedup 估计：**1.3–1.5×**

---

## 4. 总结与推荐

| 方向 | 难度 | 预估收益 | 风险 | 实现 SL OC |
|---|---|---|---|---|
| A. Loop 迭代 memoization | 中 | **10–20×** | 低（cache 正确性） | 大 |
| B. Table 数据 cache | 低 | 1.05× | 极低 | 小 |
| C. Decimal 分配优化 | 高 | 1.1× | 高（依赖 3rd party） | 大 |
| D. 禁 engine 内部 DAG 并行 | 低 | 1.3–1.5× | 低 | 小 |

### 4.1 强烈推荐方向 A

它**从根上改变了游戏规则**：不是让 8 核跑得更快，而是让需要跑的总工作量减少
~50×。对 task #033 这种"只变 n"的 batch 尤其有效；对其他类型的 batch（参数
完全随机）则无效——但即使无效也不会更慢。

### 4.2 同时推荐方向 B + D

两个都是小改动、低风险、独立收益，可以和方向 A 并行推进。合计再加 1.3–1.6×。

### 4.3 不推荐方向 C

收益小、改动大、跨库风险高。将来如果决定换 decimal 库再考虑。

---

## 5. 待确认的设计问题

实施方向 A 之前需要用户确认：

1. **是否接受 ResultCache 规模提升？** 方向 A 会让 cache entry 数暴增（每个
   loop 迭代一个 entry）。可能需要：
   - 把默认 `cacheSize` 从 1000 提升到 10000+
   - 或新增一个独立的"sub-formula result cache"，和顶层结果分开管理

2. **Fold loop 是否放弃优化？** Fold 模式（有状态累积）无法 memoize 迭代——只
   能按原方式跑。这会使方向 A 的收益集中在 sum/product/count/avg/min/max 模式
   上。是否接受？

3. **Cache eviction 策略？** 当前是 LRU。对 batch test 场景，"一次 batch 内
   频繁使用的 sub-formula 结果"应该保留，"其他 batch 的结果"可以淘汰——
   LRU 刚好满足。不需要改。

4. **是否要一起做方向 B + D？** 两者独立，但一起做可以一次性收尾性能优化
   这条线。

---

## 6. 如何重现本分析

### 测量 effective parallelism

```python
# 伪代码
clearCache()
setLimit(limit_value)
resp = POST /api/v1/calculate/batch-test with 100 cases
wall = resp.summary.totalExecutionTimeMs
sum_case = sum(c.executionTimeMs for c in resp.results)
effective = sum_case / wall
```

### 验证是 CPU-bound 而非 lock-bound

- 如果 effective < workers：lock contention（没看到）
- 如果 effective ≈ workers 但 per-case 平均耗时上涨：CPU saturation（当前情况）
- 如果 effective ≈ workers 且 per-case 平均耗时不变：完美 scaling
  （方向 A 可能做到）

### 交叉验证 workload 类型

- 纯算术公式（`财产险保费计算`）：6ms @ 1 worker → 2ms @ 8 workers，plateau
- loop + table 公式（`定期保険一時払純保険料`）：10s @ 1 worker → 4s @ 8 workers，plateau
- 两者都 plateau，证明瓶颈是共通的（CPU cores），不是特定公式的锁
