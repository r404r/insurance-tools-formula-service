# 001 - 用户管理实现

**Status**: completed
**Created**: 2026-04-06

## 需求

实现完整的用户管理功能，包括：
- 管理员可以查看所有用户列表
- 管理员可以修改用户角色（admin/editor/reviewer/viewer）
- 管理员可以删除用户（不能删除自己）

后端已有 List 和 UpdateRole，需补充 Delete；前端缺少用户管理页面。

## 设计

**后端**（最小改动）：
- `UserRepository` 接口添加 `Delete(ctx, id)`
- SQLite 实现：`DELETE FROM users WHERE id = ?`，防止删除最后一个 admin
- `UserHandler.Delete()`：防止自删、返回 204
- 路由：`DELETE /api/v1/users/{id}`

**前端**：
- `src/api/users.ts` - API 封装（listUsers, updateUserRole, deleteUser）
- `src/components/shared/UserManagementPage.tsx` - 表格形式列出用户，下拉修改角色，删除按钮
- `App.tsx` - 添加 `/users` 路由
- i18n - 三语言添加 `user.*` key

## TODO

- [x] 创建任务文件
- [x] 后端：UserRepository 接口添加 Delete
- [x] 后端：SQLite userRepo 实现 Delete
- [x] 后端：UserHandler 添加 Delete handler
- [x] 后端：路由添加 DELETE /api/v1/users/{id}
- [x] 前端：创建 src/api/users.ts
- [x] 前端：创建 UserManagementPage.tsx
- [x] 前端：更新 App.tsx 路由
- [x] 前端：更新三语言 i18n 文件

## 已知限制

- **JWT 未立即吊销**：删除用户后，其现有 token 在过期前仍然有效（无状态 JWT 的固有限制）。
  完整修复需要 token 黑名单（Redis/DB），复杂度较高，作为后续工作处理。
  当前缓解：token 默认过期时间较短；被删用户调用 `/auth/me` 时会立即返回 404。
