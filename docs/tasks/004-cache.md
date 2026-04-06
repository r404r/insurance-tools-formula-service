# 004 - 缓存功能实现

**Status**: completed
**Created**: 2026-04-07

## 需求

- 同一公式 + 同一输入 → 第二次计算直接返回缓存结果（跳过引擎计算）
- 管理页面提供「缓存统计」和「清除缓存」功能

## 设计

### 缓存键策略

`Calculate()` 只接收 graph（无 formulaID/version）。用 SHA-256(graphJSON) + SHA-256(sorted inputs) 作为复合键，天然支持同一公式不同版本（graph 内容不同 → hash 不同）。表数据变更后用「清除缓存」手动失效。

### 后端

- `engine/engine.go`：
  - `Calculate()` 方法：开头 `cache.Get()`，计算后 `cache.Set()`
  - `Engine` 接口新增 `ClearCache()` 和 `CacheStats() (size, maxSize int)`
- `api/calc_handler.go`：`CalculationEngine` 接口同步新增两方法
- 新建 `api/cache_handler.go`：
  - `GET /api/v1/cache` → 返回 `{size, maxSize, hitRate}`
  - `DELETE /api/v1/cache` → 清空缓存（需要 PermAdmin）
- `api/router.go`：注册路由

### 前端

- `api/cache.ts`：`getCacheStats()`、`clearCache()`
- 新建 `components/shared/CacheSettingsPage.tsx`：显示统计 + 清除按钮（admin only）
- `App.tsx`：添加 `/cache` 路由
- i18n：zh/en/ja 添加 `cache.*` keys

## TODO

- [x] 创建任务文件
- [x] 后端：Engine 接口 + defaultEngine 实现 ClearCache/CacheStats
- [x] 后端：Calculate() 接入缓存
- [x] 后端：CalculationEngine 接口同步
- [x] 后端：cache_handler.go + 路由
- [x] 前端：api/cache.ts
- [x] 前端：CacheSettingsPage.tsx
- [x] 前端：App.tsx 路由 + Nav 入口
- [x] 前端：i18n
- [x] codex review + fix
- [x] 提交
