# Task #028: 公式复制功能

## Status: done

## 需求

公式一览页面每行追加 Copy 按钮，点击后弹出命名框（预填 `原名 (Copy)` + 原描述），确认后直接跳转到新公式的编辑页。

## 设计

### 后端
- 新增 `POST /api/v1/formulas/:id/copy`，权限 `PermFormulaCreate`
- Request body: `{"name": string, "description": string}`（两者都可选，不传则使用默认值）
- 处理逻辑（原子操作）：
  1. 获取源 formula（404 if not found）
  2. 获取源 formula 的最新版本（404 if no versions）
  3. 创建新 formula（同 domain，新 id，新 name/description）
  4. 创建新 version v1，graph = 源版本的 graph 副本，state = draft
  5. 返回新 formula（含 id）
- 文件：`backend/internal/api/formula_handler.go`

### 前端
- `FormulaList.tsx`：
  - Actions 列添加 Copy 按钮（Editor+ 可见）
  - 弹出 modal，预填「原名 (Copy)」+ 原描述
  - 提交后 POST /formulas/:id/copy，导航到新公式编辑页
- i18n：`formula.copy`, `formula.copyTitle`, `formula.copyHint` (en/zh/ja)

## TODO

- [x] 后端 handler + 路由 (POST /formulas/:id/copy)
- [x] 前端 Copy 按钮 + modal + navigate
- [x] i18n 三语 (copy, copyTitle, copyHint, copySuffix)
- [x] codex review (6 issues, all fixed)
- [x] 全量测试通过 (backend + 118 frontend)

## Codex Review Findings Fixed

1. ✅ **Copy latest version, not published**: removed state filter in version selection
2. ✅ **Transaction**: on version creation failure, delete the formula shell; if delete also fails, return error with clear message
3. ✅ **Error mapping**: ListVersions error → 500 (not 400)
4. ✅ **Description pointer**: use `*string` to distinguish nil (use source) from `""` (intentionally cleared)
5. ✅ **Cancel button during pending**: disabled while mutation in flight; added error feedback in modal
6. ✅ **i18n copy suffix**: added `formula.copySuffix` key for zh/en/ja so default name localizes
