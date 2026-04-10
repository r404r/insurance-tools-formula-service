# Task #034: 新拖入节点无法连线的 stale closure 修复

## Status: in-progress

## 需求

用户报告：在可视化编辑器里拖入一个新的 Round 函数节点后，
从已有节点（例如 `op_result`）的 out 端口拖线到新节点的 in 端口，
**松手后连线就消失了**，连接失败。

临时绕过方法：切到 Text Editor 再切回 Visual Editor，就能连上了。

**复现公式**：`eb0931f6-48c3-4ede-b62f-ce3048a9a958`（定期保険一時払純保険料 (round)）

## 分析

根因在 `FormulaCanvas.tsx` 里两处不规范的 React 状态更新模式：

### 问题 1：`handleNodesChange` 使用闭包 `nodes` 而非 functional setState

```tsx
const handleNodesChange: OnNodesChange = useCallback(
  (changes) => {
    const updated = applyNodeChanges(changes, nodes)  // ← 闭包 nodes 可能 stale
    onNodesChange(updated)
  },
  [nodes, onNodesChange]
)
```

React Flow 官方示例的规范写法是：
```tsx
setNodes((nds) => applyNodeChanges(changes, nds))
```

### 问题 2：`handleDrop` 用 `[...nodes, newNode]` 绕过 change 系统

```tsx
onNodesChange([...nodes, newNode])  // ← 闭包 + 非 'add' change
```

### 为什么会导致"新节点无法连线"

新节点从 drop 到可连线有几步异步：

1. `handleDrop` → setNodes([老 N 个, 新 round])
2. React re-render → `adoptUserNodes` 登记新 round，`handleBounds = undefined`
3. `NodeWrapper` mount → `ResizeObserver.observe(新 round DOM)`
4. (async microtask) ResizeObserver 回调 → `updateNodeInternals` 测量 →
   写 `handleBounds` → push `dimensions` change → `triggerNodeChanges([dimChange])` →
   调用最新 `handleNodesChange`

如果此时同一 tick 内发生其它触发 `handleNodesChange` 的事件（例如 selection
change、或者 position change），而 `handleNodesChange` 的闭包 `nodes` 还是
上一个版本（没有新 round 或者新 round 没有 measured），那么
`applyNodeChanges(changes, 闭包_nodes)` 会把新 round 的 measured 丢失或把
整个节点从用户 state 里抹掉。

结果：新节点在 `react-flow` 内部 `nodeLookup` 里可能被部分覆盖回没有
`handleBounds` 的状态，导致：

- `XYHandle.onPointerDown` 从 `nodeLookup` 拿不到新节点的 `in` handle
- 或者 `getClosestHandle` 搜索不到任何候选 handle
- 连线无法"锁定"目标 → 松手后消失

切 Text → 切回 Visual 会 unmount/remount 整个 `<FormulaCanvas>`，ReactFlow
实例完全重建，`adoptUserNodes` 从零跑一次所有节点，`ResizeObserver` 全部
重新 observe，`handleBounds` 全部干净地重建 → 连线正常。

## 设计

### 修复 1：`handleNodesChange` 改 functional setState

```tsx
const handleNodesChange: OnNodesChange = useCallback(
  (changes) => {
    onNodesChange((prev) => applyNodeChanges(changes, prev))
  },
  [onNodesChange]
)
```

### 修复 2：`handleDrop` 通过 `applyNodeChanges` 的 `'add'` change 类型

```tsx
const newNode: Node = { id: nextId(), type: 'formulaNode', position,
                        data: createNodeData(type, defaultNodeConfig(type)) }
onNodesChange((prev) => applyNodeChanges([{ type: 'add', item: newNode }], prev))
```

### 类型调整

`FormulaCanvas` 的 `onNodesChange` props 类型目前是
`(nodes: Node[]) => void`，不支持 functional form。需要改成：

```tsx
onNodesChange: React.Dispatch<React.SetStateAction<Node[]>>
onEdgesChange: React.Dispatch<React.SetStateAction<Edge[]>>
```

这样 functional 调用 `onNodesChange((prev) => ...)` 能直接传递给父组件的
`setNodes`（`FormulaEditorPage.tsx:616` 本来就是 `onNodesChange={setNodes}`）。

### 同步修复 `handleEdgesChange`

一并把 `handleEdgesChange` 也改成 functional 形式，避免同类问题在 edges
侧复现：

```tsx
const handleEdgesChange: OnEdgesChange = useCallback(
  (changes) => {
    onEdgesChange((prev) => applyEdgeChanges(changes, prev))
  },
  [onEdgesChange]
)
```

## 涉及文件

- `frontend/src/components/editor/FormulaCanvas.tsx`（主要修改）
- `frontend/src/components/editor/FormulaEditorPage.tsx` 可能需要小调整
  （把 `setNodes`/`setEdges` 直接透传即可，实际已经是这样）

## TODO

- [x] 分析根因
- [x] 用户确认方案 A
- [x] 修改 `FormulaCanvas` props 类型为 `Dispatch<SetStateAction<...>>`
- [x] 修改 `handleNodesChange` 为 functional form
- [x] 修改 `handleEdgesChange` 为 functional form
- [x] 修改 `handleDrop` 为 functional form + `applyNodeChanges` 'add' change
- [x] 修改 `handleConnect` 为 functional form（顺带）
- [x] `npm run build` 验证：通过
- [x] 浏览器冒烟测试：干净 v5 版本下拖入 function 节点 → 节点正常落盘、有 dimensions、有 in handle
- [x] `/codex review`：无 findings
- [ ] 用户人工验证：在真实浏览器里复现原 bug 场景（drop → 立刻连线），确认修复生效
- [ ] commit

## 完成标准

- [ ] TypeScript 类型编译通过
- [ ] 干净状态下拖入新节点后立即能从 `op_result.out` 连线到新节点的 `in`
- [ ] 无需切换 Text/Visual 模式作为 workaround
- [ ] 其它交互（拖动、选择、删除）不受影响
