# 006 - 并发控制与 DB 连接池

**Status**: done
**Created**: 2026-04-07

## 需求

保费计算接口需要支持大批量并发访问，同时需要防止并发过载：

1. 检查当前并发设计（线程安全性、DB 连接池、并发上限）
2. 为 `/api/v1/calculate*` 添加最大并发量控制，超过上限返回 503
3. 为 SQLite 设置合理的连接池上限，防止 "database is locked" 错误

## 设计

### 并发分析结论

| 组件 | 状态 | 说明 |
|------|------|------|
| `ResultCache` | 安全 | `sync.RWMutex` 保护所有读写 |
| `Executor` | 安全 | `sync.Map` + `errgroup` goroutine 安全 |
| HTTP handlers | 安全 | 无共享可变状态 |
| DB 连接池 | 待修复 | 默认无上限，高并发下可能 "database is locked" |
| 并发上限 | 缺失 | 无信号量 / 限流中间件 |
| `BatchCalculate` | 顺序执行 | 内部串行，但每次 Calculate 本身并行 |

### 实现方案

1. **`ConcurrencyLimiter` 中间件**（`api/middleware.go`）
   - 使用 buffered channel 作为信号量
   - 满载时返回 `503 + Retry-After: 1`
   - 配置项：`ENGINE_MAX_CONCURRENT_CALCS`（默认 100）

2. **SQLite 连接池**（`store/sqlite/store.go`）
   - `SetMaxOpenConns(10)` — WAL 模式下合理上限
   - `SetMaxIdleConns(5)`
   - `SetConnMaxLifetime(time.Hour)`

3. **配置**（`config/config.go`）
   - `EngineConfig.MaxConcurrentCalcs int`
   - `envInt("ENGINE_MAX_CONCURRENT_CALCS", 100)`

4. **路由**（`api/router.go`）
   - `RouterConfig.MaxConcurrentCalcs int`
   - `/calculate` 路由组注入 `ConcurrencyLimiter(cfg.MaxConcurrentCalcs)`

## TODO

- [x] 创建任务文件
- [x] 并发设计分析
- [x] config：添加 `MaxConcurrentCalcs` 字段
- [x] middleware：实现 `ConcurrencyLimiter`
- [x] router：应用中间件到 `/calculate` 路由组
- [x] sqlite store：设置连接池上限
- [x] main.go：传入 `MaxConcurrentCalcs`
- [x] 编译验证
- [x] 提交
