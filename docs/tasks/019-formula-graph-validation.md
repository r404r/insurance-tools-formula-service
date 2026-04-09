# Task #019: 公式图结构合理性校验增强

## Status: done

## 需求

公式编辑器保存时，从公式图的结构层面校验合理性，包括：

1. **环路检测**：DAG 不允许有环（否则引擎会死循环）。现有前端只做端口/连接校验，没有检测环路。
2. **孤立节点检测**：不在任何输出路径上的节点是「死代码」，应警告用户。
3. **多错误定位**：将验证结果从 `string | null` 升级为结构化类型，包含受影响的 nodeId，以便在 Canvas 上高亮显示。
4. **节点高亮**：Canvas 中对有问题的节点加红色/黄色边框，直观定位错误位置。
5. **后端深度校验**：新增 `POST /api/v1/validate` 端点，对图结构运行完整 engine.Validate()，保存前调用。

## 设计

### 验证结果类型

```ts
interface ValidationIssue {
  message: string
  nodeIds: string[]      // 空数组 = 全局问题
  severity: 'error' | 'warning'
}
```

`validateGraph` 返回 `ValidationIssue[]`（空数组 = 合法）。

### 环路检测（前端，DFS）

```
coloring: white(未访问) | gray(处理中) | black(完成)
对每个节点运行 DFS；gray 节点的后继如果也是 gray，说明有环。
收集环路节点的 ID。
```

### 孤立节点检测（从输出逆向 BFS）

```
从所有输出节点出发，顺着入边反向遍历；
未被访问到的节点 = 孤立节点（警告级别）。
```

### 节点高亮

`FormulaCanvas` 接收 `invalidNodeIds: string[]` 和 `warnNodeIds: string[]` prop，
在 `FormulaNode` 的样式中加红/黄边框。

### 后端 validate 端点

```
POST /api/v1/validate
Body: { "graph": FormulaGraph }
Response: { "valid": bool, "errors": [{"nodeId": "...", "message": "..."}] }
```

实现：直接调用 `engine.NewEngine(cfg).Validate(graph)` 并返回结果。
不需要数据库，纯计算。

### 保存流程变化

```
handleSave():
  1. validateGraph(nodes, edges) → issues
     有 error → 停止，高亮节点，显示错误
     有 warning → 允许继续，但显示警告
  2. POST /api/v1/validate { graph } → 后端深度校验
     有 error → 停止，显示错误
  3. POST /formulas/{id}/versions → 保存
```

## 涉及文件

**后端：**
- `backend/internal/api/handlers.go`（或新建 validate handler）
- `backend/internal/api/router.go`（注册路由）

**前端：**
- `frontend/src/components/editor/FormulaEditorPage.tsx`（validateGraph 重构 + 保存逻辑）
- `frontend/src/components/editor/FormulaCanvas.tsx`（接收高亮 props）
- `frontend/src/components/editor/FormulaNode.tsx`（高亮样式）
- `frontend/src/api/`（新增 validateGraph API）

## TODO

- [x] 创建 task 文件
- [ ] 后端：新增 `POST /api/v1/validate` 端点
- [ ] 后端：编写端点单元测试
- [ ] 前端：重构 `validateGraph` 返回类型为 `ValidationIssue[]`
- [ ] 前端：实现环路检测（DFS 三色染色）
- [ ] 前端：实现孤立节点检测（从输出逆向 BFS）
- [ ] 前端：更新 `handleSave` 调用后端 validate 端点
- [ ] 前端：`FormulaCanvas` 接收 `invalidNodeIds` / `warnNodeIds` props
- [ ] 前端：`FormulaNode` 根据高亮 props 加红/黄边框
- [ ] 前端：更新错误展示 UI（支持多条错误信息）
- [ ] 验证：手动测试环路、孤立节点、正常公式场景
- [ ] codex review + commit

## 完成标准

- [ ] 创建含环路的公式 → 保存时前端立即报错并高亮环路节点
- [ ] 创建含孤立节点的公式 → 保存时警告并高亮孤立节点
- [ ] 正常公式 → 保存成功
- [ ] 后端 validate 接口对非法图返回结构化错误
- [ ] 所有后端测试通过
