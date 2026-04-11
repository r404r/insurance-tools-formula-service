# Task #037: Lookup Table 数据缓存（方向 B）

## Status: done

<!-- planning | in-progress | blocked | done -->

## 需求

`docs/performance/001-batch-test-speedup-analysis.md` 方向 B：当前 `StoreTableResolver.ResolveTable`
每次被调用都做「SQL 查询 → JSON unmarshal → 建 map」整套动作。对 task #033
的 100 case batch test（定期保険一時払純保険料公式），同一张 qx 表会在一次
batch run 中被**重复解析约 300 次**（100 case × ~3 次 preload/case，顶层 + loop 节点）。
每次 1–3ms，合计约 300–900ms 被浪费在重复解析同一张表上。

本 task 为表数据加上按 tableID 的进程内缓存，通过现有的
`TableHandler` → `CacheInvalidator.ClearCache()` 失效路径保持一致性。

## 设计

### 缓存层级：parsed rows（`[]map[string]string`）

不缓存 `ResolveTable` 的最终返回值 `map[compositeKey]string`，而是缓存中间产物
—— 刚被 `json.Unmarshal` 出来的 raw rows。原因：

1. **跨 column / keyColumns 共享**：同一张表被不同节点用不同的 `column`/`keyColumns`
   lookup 时，rows 数据可共用，只有最后的组合步骤不同。
2. **composite key 构建很便宜**：只是字符串拼接 + map insert，没 DB、没 JSON、没 decimal parse。
3. **缓存条目少**：每张表一个条目，内存占用最小。

### 并发安全

- 使用 `sync.RWMutex` 保护 cache map。
- 缓存 miss 时用 `golang.org/x/sync/singleflight.Group` 去重并发加载
  （已在 `go.mod` 中），避免两个 goroutine 同时 miss 同一张表时做重复工作。
- **Cached rows 是只读共享**：只有 `ResolveTable` 读它，并且只是迭代构建新的 result map，
  从不 mutate。文档化这一点以防未来误用。

### 失效策略

和现有 `engine.ClearCache()` 一起失效：

1. `StoreTableResolver` 暴露 `InvalidateAll()` 和 `Invalidate(tableID string)` 方法。
2. `engine.defaultEngine.ClearCache()` 在清空 `ResultCache` 之后，也调用
   `tableResolver.InvalidateAll()`（通过 interface 类型断言，保持向后兼容：
   非 StoreTableResolver 的自定义实现不会被影响）。
3. `TableHandler.Update/Delete` 已经调用 `h.Cache.ClearCache()`——这条路径会
   经由 engine 自动级联到 table resolver cache，无需改 handler。
4. Admin 的 "Clear Cache" 按钮也走 `engine.ClearCache()`，同样级联。

**不走 UpdatedAt 路径**：domain.LookupTable 没有 UpdatedAt 字段，schema 也没有
`updated_at` 列。走手动失效和现有 cache 的一致性模型保持一致，避免 schema 迁移。

### 涉及文件

- `backend/internal/engine/table_resolver.go` — 改 `StoreTableResolver` 加缓存
- `backend/internal/engine/engine.go` — `ClearCache()` 级联到 tableResolver
- `backend/internal/engine/table_resolver_test.go` — 新建，测试 cache hit/miss/invalidate/并发
- `backend/go.mod` — `golang.org/x/sync/singleflight` 已存在，无需修改

## TODO

- [x] 修改 `table_resolver.go`：给 `StoreTableResolver` 加 rows cache + singleflight
- [x] 新增 `InvalidateAll()` / `Invalidate(tableID)` 方法
- [x] 修改 `engine.go` 的 `ClearCache()`：类型断言 + 级联 InvalidateAll
- [x] 写 `table_resolver_test.go`：基本命中、失效、并发 singleflight、并发 mutation 安全
- [x] `go vet ./... && go test ./... -race` 通过
- [ ] 手动验证：batch test 前后对比耗时（需重启现有 dev server）
- [x] codex review → 修复 P1（失效期间的加载竞态）+ P2（取消传染） → commit a00a9ee
- [x] 更新 `docs/backlog.md` 和 `docs/performance/001-batch-test-speedup-analysis.md`

## 完成标准

- [x] 功能正常：冷缓存首次查询走 DB，之后走缓存
      （`TestStoreTableResolver_CachesParsedRows` 冷/热/跨 column 三种路径断言）
- [x] 并发安全：`go test -race` 通过
      （所有 7 个 resolver 测试 + 全 backend 套件在 `-race` 下通过）
- [x] 失效正确：TableHandler Update/Delete 触发的 ClearCache 级联清掉 table cache
      （`engine.defaultEngine.ClearCache()` 显式 interface 断言后调用
      `InvalidateAll`；`TableHandler.Update/Delete` 已调用 `h.Cache.ClearCache()`；
      `InvalidateAllForcesReload` + `InvalidateDuringLoadDoesNotResurrectStale`
      覆盖两条链路）
- [x] 测试通过（race + 单元）
      （见上；codex review 两轮后全绿，commit a00a9ee）
- [x] commit + codex review
      （a00a9ee；codex round 1 发现 P1/P2 → 修复后 round 2 LGTM）
