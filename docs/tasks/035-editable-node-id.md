# Task #035: 可视化编辑器支持修改节点 ID

## Status: done

## 需求

在图形编辑器的属性面板里，节点 ID 目前是只读的（`NodePropertiesPanel.tsx:75`
是 `disabled` input）。用户希望能够自定义每个节点的 ID，方便后续在 Text Editor、
错误消息、调试时识别节点。

编辑时必须：
- 检测 **ID 冲突**（与当前 graph 中其他节点 ID 重名）
- 检测 **ID 格式非法**（空字符串、非 identifier 字符等）
- 给出直观的警告提示
- 确认合法后，原子更新节点 id + 所有指向它的边的 source/target

## 设计

### ID 合法性规则

| 规则 | 值 |
|---|---|
| 非空 | 必须 |
| 首字符 | `[A-Za-z_]` |
| 后续字符 | `[A-Za-z0-9_]` |
| 最大长度 | 64 |
| 正则 | `^[A-Za-z_][A-Za-z0-9_]{0,63}$` |

历史遗留的 ID（例如用 `-` 或其他字符的）**不强制迁移**，只有在用户主动编辑时
才应用新规则，避免破坏现有公式。

### 编辑 UX

- `NodePropertiesPanel` 的 ID input 去掉 `disabled`
- 本地 draft state，输入时实时校验但不立即 commit
- 实时错误反馈：
  - 空 → `editor.idEmpty`
  - 格式非法 → `editor.idInvalid`
  - 与其他节点冲突 → `editor.idConflict`
- **提交时机**：blur 或 Enter
- 提交合法 → 原子 rename
- 提交非法 → 回退 input 值到当前有效 ID + 保留错误提示一次
- 提交时 ID 未变 → 不做任何操作

### 原子 rename 实现

新增 `FormulaEditorPage.handleNodeIdChange(oldId, newId)`：

```ts
const handleNodeIdChange = useCallback((oldId: string, newId: string) => {
  if (oldId === newId) return
  setNodes((prev) => prev.map((n) => (n.id === oldId ? { ...n, id: newId } : n)))
  setEdges((prev) =>
    prev.map((e) => ({
      ...e,
      source: e.source === oldId ? newId : e.source,
      target: e.target === oldId ? newId : e.target,
    }))
  )
  setSelectedNodeId((cur) => (cur === oldId ? newId : cur))
}, [])
```

`outputs` 不用显式管理 — `handleSave` 在保存前从 `nodes`+`edges` 派生。

### 涉及文件

- `frontend/src/utils/nodeIdValidation.ts`（新建）
  - 导出 `NODE_ID_REGEX` 和 `validateNodeIdFormat(id: string): 'ok' | 'empty' | 'invalid'`
- `frontend/src/components/editor/NodePropertiesPanel.tsx`
  - 新增 props `onIdChange` + `existingNodeIds: Set<string>`
  - 替换 ID input 实现为受控 + 校验
- `frontend/src/components/editor/FormulaEditorPage.tsx`
  - 新增 `handleNodeIdChange`
  - 构造 `existingNodeIds` 传给 NodePropertiesPanel
- `frontend/src/i18n/locales/{en,zh,ja}.json`
  - 新增 `editor.idEmpty` / `editor.idInvalid` / `editor.idConflict`

## TODO

- [x] 用户确认方案 A
- [x] 创建 `nodeIdValidation.ts`
- [x] 添加 i18n keys (en/zh/ja)
- [x] 修改 `NodePropertiesPanel` 的 ID 字段
- [x] 添加 `handleNodeIdChange` 到 `FormulaEditorPage`
- [x] `npm run build` 验证
- [x] 浏览器冒烟测试：
  - rename 成功（op_result → final_result，边同步更新）✓
  - 空、digit-start、hyphen、conflict 四种错误提示都正确触发 ✓
- [x] `/codex review` → P2 修复：`commitIdChange` 不再 trim，原始输入直接参与校验
  - 补测：输入 `new_id ` 带尾部空格，格式错误提示正确弹出，commit 被拒
- [x] commit

## 完成标准

- [ ] 在可视化编辑器里可以编辑 ID
- [ ] 空、非法、冲突三种错误都有清晰提示
- [ ] 合法 rename 后：节点 id 变了、所有指向它的边 source/target 同步更新、
      可以正常保存
- [ ] 保存后重新加载公式，改名后的 ID 保留
- [ ] 历史遗留 ID 不触发强制迁移
