# Task #025: 公式删除功能 + 预置数据重置

## Status: done

## 需求

1. 公式删除功能：仅 admin 可操作，删除前需确认对话框
2. Settings 页面追加"重置预置公式及料率表"按钮：删除预置数据并重新生成，不影响用户自行创建的内容

## 设计

### 功能 1：公式删除

**后端（已有，需调整）：**
- DELETE `/api/v1/formulas/{id}` 路由和 handler 已存在
- 需修改：`auth/rbac.go` 将 `PermFormulaDelete` 从 Editor 角色移除，仅保留 Admin

**前端（新增）：**
- `FormulaList.tsx`：admin 用户在公式行上显示删除按钮
- 点击后弹出确认对话框（使用 `window.confirm` 或自定义 modal）
- 确认后调用 `api.delete('/formulas/{id}')`
- 删除成功后刷新列表

### 功能 2：预置数据重置

**后端（新增）：**
- 新增 POST `/api/v1/admin/reset-seed` 端点，PermUserManage（admin only）
- 逻辑：
  1. 读取 seed 公式名称列表（硬编码在 seed 函数中的名称）
  2. 按名称匹配删除预置公式（cascade 删除版本）
  3. 按名称匹配删除预置料率表
  4. 重新执行 seed() 函数
- 不删除用户自建公式/料率表（通过名称匹配区分）

**前端（新增）：**
- `AdminSettingsPage.tsx` 追加"预置数据"区块
- 按钮"重置预置公式及料率表"
- 确认对话框（warn 级别）
- 成功后显示提示

## TODO

### 后端
- [x] `auth/rbac.go`: 从 Editor 移除 PermFormulaDelete（仅 Admin 可删）
- [x] `cmd/server/main.go`: seedFormulaNames/seedTableNames 常量 + makeSeedResetHandler（含 CreatedBy 校验）
- [x] `api/router.go`: POST /admin/reset-seed 路由（PermUserManage）

### 前端
- [x] `FormulaList.tsx`: admin 用户显示删除按钮 + window.confirm 确认
- [x] `AdminSettingsPage.tsx`: SeedResetSection 组件 + 确认对话框
- [x] i18n: en/zh/ja 三语新增 deleteConfirm、seedSection/seedHint/seedReset/seedResetConfirm/seedResetSuccess

### 测试
- [x] codex review（5 issues，已修复 #1 provenance check）
- [x] 全量测试通过
