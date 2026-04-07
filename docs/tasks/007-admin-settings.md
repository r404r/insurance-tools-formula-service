# Task #007: 管理员系统设置页面

## Status: done

## 需求

为 Admin 角色追加系统设置管理界面，支持通过 UI 设置「最大并发计算数」，修改即时生效，无需重启服务。

## 设计

### 后端

- **持久化**：新增 `settings` 表（key-value），`SettingsRepository` 接口 + SQLite 实现
- **动态并发限制器**：将 task-006 的静态 `ConcurrencyLimiter` 升级为 `DynamicConcurrencyLimiter`，`SetLimit(n)` 在运行时原子替换 semaphore channel，in-flight 请求安全完成后旧 channel 被 GC
- **API**：
  - `GET /api/v1/settings` — 返回当前设置（admin-only）
  - `PUT /api/v1/settings` — 更新设置，持久化 + 即时应用（admin-only）
- **启动恢复**：服务启动时从 DB 读取 `max_concurrent_calcs`，覆盖 env var 默认值

### 前端

- `src/api/settings.ts` — `getSettings()` / `updateSettings()`
- `src/components/shared/AdminSettingsPage.tsx` — 数字输入框 + 保存按钮 + inline 成功提示
- `src/App.tsx` — 新增 `/admin/settings` 路由
- `src/components/shared/Navbar.tsx` — Admin 菜单追加「系统设置」入口
- i18n：zh/en/ja 追加 `adminSettings.*` + `nav.adminSettings`

## 涉及文件

- `backend/internal/store/repository.go`
- `backend/internal/store/sqlite/store.go`
- `backend/internal/store/sqlite/settings.go`（新）
- `backend/internal/api/middleware.go`
- `backend/internal/api/settings_handler.go`（新）
- `backend/internal/api/dto.go`
- `backend/internal/api/router.go`
- `backend/cmd/server/main.go`
- `frontend/src/api/settings.ts`（新）
- `frontend/src/components/shared/AdminSettingsPage.tsx`（新）
- `frontend/src/App.tsx`
- `frontend/src/components/shared/Navbar.tsx`
- `frontend/src/i18n/locales/zh.json`, `en.json`, `ja.json`

## TODO

- [x] 创建任务文件
- [x] store：SettingsRepository 接口
- [x] store/sqlite：settings 表 + settingsRepo 实现
- [x] api：DynamicConcurrencyLimiter（替换静态版本）
- [x] api：SettingsHandler（GET + PUT）
- [x] api：SettingsResponse / UpdateSettingsRequest DTOs
- [x] router：注册 /settings 路由（admin-only）
- [x] main.go：启动时加载持久化设置，wire settingsHandler
- [x] 前端：settings.ts API 客户端
- [x] 前端：AdminSettingsPage.tsx
- [x] 前端：App.tsx 路由
- [x] 前端：Navbar 入口（admin-only）
- [x] 前端：i18n（zh/en/ja）
- [x] 编译验证（go build + tsc）
- [x] codex review + fix P1/P2
- [x] 提交

## 完成标准

- [x] 功能正常
- [x] 测试通过
- [x] commit + codex review
