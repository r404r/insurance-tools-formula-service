# Task #042: 公式列表加作成者/更新者 + 表头排序

## Status: done

## 需求

实现 [`docs/specs/005-sortable-formula-list.md`](../specs/005-sortable-formula-list.md)：

1. 公式列表新增 `createdBy` / `updatedBy` 列（显示用户名，不是 UUID）
2. 表头可点击触发排序（除「操作」列），按 `name / createdAt / updatedAt / createdBy / updatedBy` 排序
3. 排序状态走 URL 参数，与现有 search/page 一致
4. 默认 `updatedAt desc`（保持当前行为）
5. 「更新者」语义：最近一次保存版本的用户（不是 metadata 编辑者）
6. NULL 显示 `—`

## TODO

### 后端
- [x] `domain.Formula` 加 `UpdatedBy / CreatedByName / UpdatedByName` 字段
- [x] `domain.FormulaFilter` 加 `SortBy / SortOrder`
- [x] `store.FormulaRepository` 接口加 `UpdateMeta(ctx, id, updatedBy, updatedAt)`
- [x] SQLite store: `Migrate()` 加 `updated_by` 列 + ALTER 容错；`List` SQL JOIN users + 动态 ORDER BY 白名单；`UpdateMeta` 实现
- [x] Postgres store: 同上（用 `ADD COLUMN IF NOT EXISTS` + `NULLS LAST`）
- [x] MySQL store: 同上（容忍 1060/1061/1826 错误码 + `(col IS NULL)` 排序技巧）
- [x] `formula_handler.go` List handler 解析 + 验证 sortBy/sortOrder query 参数
- [x] `version_handler.go` Create version 后调用 `Formulas.UpdateMeta`
- [x] **codex round 1 P1**：`Copy` 和 `Import` handler 也加 `UpdateMeta`
- [x] **codex round 2 P2**：`main.go` 的 `seedFormula` 也加 `UpdateMeta`
- [x] 单元测试：sort 各字段 asc/desc、白名单拒绝非法值、ALTER 幂等性、UpdateMeta 路径（11 sub-tests）

### 前端
- [x] `types/formula.ts` 加 `updatedBy / createdByName / updatedByName` + `FormulaSortField` / `SortOrder` 类型
- [x] `FormulaList.tsx` 表头改为可点击 `SortableTh` 组件
- [x] 加 createdBy / updatedBy / updatedAt 三列
- [x] queryKey 加 sortBy/sortOrder
- [x] API 调用带 sort 参数
- [x] **codex round 1 P2**：sort 状态走 `useSearchParams` URL 参数（spec 要求刷新不丢）
- [x] i18n 三语 keys

### 验证
- [x] `go vet ./... && go test ./... -race` 通过
- [x] `npm run build` 通过
- [x] codex review 三轮 → P1+P2×2 全部修复 → LGTM
- [ ] 浏览器冒烟（手动）
- [x] 移到 backlog 已完成

## 完成标准

- [ ] 列表显示 createdBy / updatedBy 用户名
- [ ] 6 个可排序字段都能正常切换 asc/desc
- [ ] 现有 List 默认行为（updatedAt desc）保持不变
- [ ] race detector + 单元测试全绿
- [ ] codex review LGTM
