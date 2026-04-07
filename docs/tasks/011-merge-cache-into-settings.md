# Task #011: Cache Management 整合到 Settings 中

## Status: done

## 需求

当前 Cache 管理（`/cache`）和系统设置（`/admin/settings`）是两个独立导航页面。
对 Admin 用户来说，这两者都是系统运维配置，合并到一个页面更合理：

- 将 Cache 统计 + 清除功能合并到 Settings 页面，作为新的「缓存管理」区块
- 删除独立的 `/cache` 路由和导航项
- `/admin/settings` 路由保持不变，内容变为两个区块：「计算引擎」+ 「缓存管理」
- 非管理员用户访问 `/admin/settings` 仍被拦截

## 设计

### 前端

- `AdminSettingsPage.tsx` — 追加「缓存管理」区块（复用 CacheSettingsPage 的逻辑）
  - 显示当前 entries / max capacity / 使用率进度条
  - 「刷新」按钮 + Admin 专属「清空缓存」按钮
  - Amber 提示栏（lookup table 更新后需手动清缓存）
- `CacheSettingsPage.tsx` — **删除**（内容并入 AdminSettingsPage）
- `App.tsx` — 删除 `/cache` 路由和 CacheSettingsPage 导入
- `Navbar.tsx` — 删除「Cache」导航项（`nav.cache`）
- i18n — 无需改动（`cache.*` 键继续在 AdminSettingsPage 中使用）

### 路由变更

| Before | After |
|--------|-------|
| `/cache` → CacheSettingsPage | 删除 |
| `/admin/settings` → AdminSettingsPage | 保留，内容扩充 |
| Navbar: Cache + Settings | Navbar: Settings only |

## 涉及文件

- `frontend/src/components/shared/AdminSettingsPage.tsx`（修改）
- `frontend/src/components/shared/CacheSettingsPage.tsx`（删除）
- `frontend/src/App.tsx`（修改）
- `frontend/src/components/shared/Navbar.tsx`（修改）

## TODO

- [x] 创建任务文件
- [x] AdminSettingsPage.tsx：追加缓存管理区块
- [x] App.tsx：/cache 改为重定向到 /admin/settings
- [x] Navbar.tsx：删除 Cache 导航项
- [x] CacheSettingsPage.tsx：删除文件
- [x] tsc --noEmit 通过
- [x] codex review + fix P1/P2
- [x] 提交

## 完成标准

- [x] 功能正常（Settings 页面显示引擎 + 缓存两个区块）
- [x] 测试通过
- [x] commit + codex review
