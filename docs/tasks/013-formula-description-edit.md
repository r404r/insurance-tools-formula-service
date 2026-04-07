# Task #013: 公式 Description 内联编辑

## Status: in-progress

## 需求

公式的 description 目前仅在创建时填写，编辑器页面无法修改。
需要在 FormulaEditorPage 的 header 中追加内联编辑功能，与现有的名称编辑保持一致的交互风格。

## 设计

### 后端

无需改动。`PUT /api/v1/formulas/{id}` 已支持 `{ description: string }` 字段。

### 前端

- `FormulaEditorPage.tsx`
  - 新增 `isEditingDesc` / `descDraft` state
  - 新增 `handleDescSave` callback（模式与 `handleNameSave` 相同）
  - Header 中名称行下方追加 description 显示行：
    - viewer/reviewer：只读显示（灰色小字，空时显示占位符）
    - editor/admin：点击进入编辑模式（inline `<textarea>` 或 `<input>`，Enter 保存，Escape 取消）
  - 保存调用 `api.put<Formula>(\`/formulas/${id}\`, { description })`，成功后 invalidate queries

### 涉及文件

- `frontend/src/components/editor/FormulaEditorPage.tsx`（修改）

## TODO

- [x] 创建任务文件
- [ ] FormulaEditorPage.tsx：追加 description 编辑状态和 handler
- [ ] FormulaEditorPage.tsx：header 追加 description 行 UI
- [ ] tsc --noEmit 通过
- [ ] codex review + fix P1/P2
- [ ] 提交

## 完成标准

- [ ] 功能正常
- [ ] 测试通过
- [ ] commit + codex review
