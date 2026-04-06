# 002 - Lookup Tables 管理实现

**Status**: in-progress
**Created**: 2026-04-06

## 需求

实现查找表（Lookup Table）的完整管理功能：
- 管理员/编辑者可以查看、新建、编辑、删除查找表
- 表数据格式：`[{"key":"...", "col1":"...", ...}, ...]`（引擎所需格式）
- 节点属性面板中 tableId 改为下拉选择（替代手动输入 ID）

## 设计

**后端**（补充 Update + Delete）：
- `TableRepository` 接口添加 `Update` 和 `Delete`
- SQLite 实现（Delete 前检查版本引用，防止孤立）
- `table_handler.go`：`Update`、`Delete` handler
- `dto.go`：`UpdateTableRequest`
- 路由：`PUT /api/v1/tables/{id}`、`DELETE /api/v1/tables/{id}`
- **权限**：读路由对所有认证用户开放（reviewer/viewer 可查看表名以理解公式节点）；写操作（create/update/delete）需要 PermTableManage（admin/editor）

**前端**：
- `api/tables.ts` — API 封装
- `TableManagementPage.tsx` — 列表 + 创建 + 编辑（JSON textarea + 预览表格）+ 删除
- `App.tsx` — 添加 `/tables` 路由
- `NodePropertiesPanel.tsx` — tableId 改为下拉（从 API 获取）
- i18n — zh/en/ja 添加 `table.*` keys

## TODO

- [x] 创建任务文件
- [ ] 后端：TableRepository 接口添加 Update/Delete
- [ ] 后端：SQLite 实现 Update/Delete
- [ ] 后端：dto.go 添加 UpdateTableRequest
- [ ] 后端：table_handler.go 添加 Update/Delete handler
- [ ] 后端：路由添加 PUT/DELETE /api/v1/tables/{id}
- [ ] 前端：创建 api/tables.ts
- [ ] 前端：创建 TableManagementPage.tsx
- [ ] 前端：更新 App.tsx 路由
- [ ] 前端：NodePropertiesPanel tableId 改为下拉
- [ ] 前端：更新三语言 i18n
- [ ] codex review + fix
- [ ] 提交
