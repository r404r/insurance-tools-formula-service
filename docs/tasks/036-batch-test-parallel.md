# Task #036: Batch Test 服务端并行化 + 总执行时间

## Status: done

## 需求

1. `POST /api/v1/calculate/batch-test` 目前是顺序 for 循环，100 个 case 一条条跑。
   改为**有限并发**并行执行。
2. 并发上限 = `max(1, floor(max_concurrent_calcs / 5))`，全局并发限流的 1/5。
   - `max_concurrent_calcs = 0`（无限）→ 使用固定默认值 8 作为 worker 上限，防止 goroutine 爆炸
   - `max_concurrent_calcs` 很小（比如 4）→ 上限仍至少为 1
3. 在 README 和 Settings 页面的 "Max Concurrent Calculations" 备注中**说明这个 1/5 关系**
4. `BatchTestSummary` 新增字段 `totalExecutionTimeMs`，前端结果汇总卡片加一个"总执行时间"显示

## 设计

### 并发上限计算

```go
// computeBatchWorkers returns the worker cap for batch-test parallelism:
// floor(globalLimit / 5), clamped to [1, batchDefaultMax]. When the global
// limit is 0 (unlimited), fall back to batchDefaultMax.
const batchDefaultMax = 8

func computeBatchWorkers(globalLimit int) int {
    if globalLimit <= 0 {
        return batchDefaultMax
    }
    w := globalLimit / 5
    if w < 1 {
        w = 1
    }
    if w > batchDefaultMax {
        w = batchDefaultMax
    }
    return w
}
```

这个 cap 来自 `CalcLimiter.Limit()`（可运行时修改）。每次请求重新读取，不缓存。

### 并行执行（errgroup + semaphore）

```go
import "golang.org/x/sync/errgroup"

results := make([]BatchTestCaseResult, len(req.Cases))
workers := computeBatchWorkers(h.CalcLimiter.Limit())
sem := make(chan struct{}, workers)

g, ctx := errgroup.WithContext(r.Context())
batchStart := time.Now()
for i, tc := range req.Cases {
    i, tc := i, tc
    g.Go(func() error {
        select {
        case sem <- struct{}{}:
        case <-ctx.Done():
            return ctx.Err()
        }
        defer func() { <-sem }()

        res, calcErr := h.Engine.Calculate(ctx, &version.Graph, tc.Inputs)
        // ...build caseResult as before...
        results[i] = caseResult
        return nil  // never return error — we record per-case failures instead
    })
}
_ = g.Wait()
totalElapsed := time.Since(batchStart)
```

关键决策：
- **结果按 index 填入数组**，保持顺序（和输入一致）
- **per-case 失败不向 errgroup 返回 error** — 我们要收集所有 case 的结果，即使某些失败
- **ctx 取消**：如果 client 断开，Goroutines 通过 `ctx.Done()` 早退
- **Engine.Calculate 是否线程安全？** 需要验证 — graph 是只读的，但 Engine 内部可能有共享状态（cache 等）。从已有的 `ParallelLevels` 字段看，单个 Calculate 调用内部已经并行 DAG 节点，所以 Engine 必然支持并发调用

### `passed++` 计数修复

原代码用 `passed++`，改并行后需要原子计数或遍历 results。**遍历 results** 更简单：
```go
passed := 0
for _, r := range results {
    if r.Pass { passed++ }
}
```

### Summary 新增字段

`backend/internal/api/dto.go`：
```go
type BatchTestSummary struct {
    Total                int     `json:"total"`
    Passed               int     `json:"passed"`
    Failed               int     `json:"failed"`
    PassRate             float64 `json:"passRate"`
    TotalExecutionTimeMs float64 `json:"totalExecutionTimeMs"`  // new
}
```

`frontend/src/types/formula.ts` 同步。

### 前端 UI

`BatchTestPage.tsx` 的 summary cards 从 4 个增加到 5 个（grid 4列 → 5列，或者换行）。新增 `totalTime` 卡片：
```tsx
{ label: t('batchTest.totalTime'), value: `${summary!.totalExecutionTimeMs.toFixed(0)} ms`, color: 'text-gray-700' }
```

grid 改为 `sm:grid-cols-5`（或者 `md:grid-cols-5`，响应式考虑）。

### 文档更新

**README.md** `## Calculation Engine` 段落：
- 把 `Concurrency Control` 描述补充：Batch Test 并行 worker 上限 = 全局上限的 1/5（无限模式下 fallback 到 8）

**Settings 页 i18n hint**（en/zh/ja 三语）：
- `maxConcurrentCalcsHint` 补一句，说明 Batch Test 并行 worker 上限 = 此值的 1/5

## 涉及文件

- `backend/internal/api/dto.go` — `BatchTestSummary` 新字段
- `backend/internal/api/batch_test_handler.go` — 并行化 + 计时
- `backend/internal/api/router.go` — `BatchTest` 路由 config 已经能访问 `CalcLimiter`（通过 `cfg`）
- `backend/go.mod` — 检查 `golang.org/x/sync/errgroup` 是否已存在
- `frontend/src/types/formula.ts` — `BatchTestSummary` 新字段
- `frontend/src/components/shared/BatchTestPage.tsx` — 新 summary card + grid 调整
- `frontend/src/i18n/locales/{en,zh,ja}.json` — `batchTest.totalTime` + 扩展 `maxConcurrentCalcsHint`
- `README.md` — Concurrency Control 描述补充

## TODO

- [x] 用户确认方案
- [x] 实现 `computeBatchWorkers` + 单元测试（10 个 case 覆盖所有边界）
- [x] 并行化 `BatchTest` handler（固定 worker pool + shared limiter per case）
- [x] DTO 新增 `totalExecutionTimeMs`
- [x] `CalcHandler.Limiter` 字段 + main.go wiring
- [x] 前端 type + UI 更新（5 个 summary card，含 `Total Time`）
- [x] i18n 三语 (en/zh/ja) — `batchTest.totalTime` + 扩展 `maxConcurrentCalcsHint`
- [x] README 文档补充 1/5 关系
- [x] `cd backend && go build ./...` + `go test ./...` 通过
- [x] `cd frontend && npm run build` 通过
- [x] 浏览器冒烟测试：UI 5 个 summary card 渲染正常
- [x] 端到端冷缓存对比：workers=1 约 10.3s，workers=8 约 4.1s，**2.52× speedup**
- [x] `/codex review` → 连续三轮修复
  - 第 1 轮：clean
  - 第 2 轮：[P1] 内层 calc 未参与全局限流、[P2] 每 case 一个 goroutine →
    修复：router 豁免 middleware，固定 worker pool，Acquire/Release 共享 limiter
  - 第 3 轮：[P1] panic 导致 server 崩溃 + slot 泄漏、[P1] SetLimit
    pre-fill + generation-captured release 交互导致新 sem 容量永久损失 →
    修复：runOneBatchCase 包 defer recover + release，SetLimit 去掉
    pre-fill，新增 generation-done channel 唤醒 blocked waiters
- [x] 新增 3 个 limiter 单元测试（TestAcquireReleaseBasic /
  TestSetLimitWakesWaiters / TestReleaseOnOldGenerationDoesNotAffectNew），
  race detector 通过
- [x] commit

## 实测数据

| 模式 | Workers | 冷缓存总耗时 (100 cases) |
|---|---:|---:|
| 顺序（limit=5） | 1 | ~10.3 秒 |
| 并行（limit=40） | 8 | ~4.1 秒 |

**Speedup: 2.52×**（未达 8× 是因为 Engine 共享状态的序列化，但单次显著改善）

## 完成标准

- [x] 100 条 case 的 batch test 并行执行，总耗时明显降低
      （实测：10.3s → 4.1s，2.52×，见上方"实测数据"表）
- [x] 结果数组顺序与输入一致
      （设计：结果按 index 写入预分配数组；`TestBatchTestConcurrentRequestsRace`
      显式断言 "case-0"..."case-199" 顺序）
- [x] Summary 包含 totalExecutionTimeMs
      （`BatchTestSummary.TotalExecutionTimeMs` 已添加到 dto.go 并由 handler 填充）
- [x] UI 显示"总执行时间"卡片
      （冒烟测试已确认 5 个 summary card 正常渲染）
- [x] 全局并发限流修改后，下一次 batch test 的 worker 数跟着调整
      （handler 每次请求都调用 `computeBatchWorkers(h.Limiter.Limit())`，
      不缓存；`TestBatchTestSetLimitMidFlightRace` 中途翻转 limit 验证 race-free）
- [x] README 和 Settings hint 明确写出 1/5 关系
      （README Concurrency Control 段 + en/zh/ja 三语 `maxConcurrentCalcsHint`）
