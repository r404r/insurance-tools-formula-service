# Task #021: Loop 节点文本模式支持

## Status: done

## 需求

解除 Loop 节点对文本模式的阻断，实现 Loop 节点的图形↔文本双向转换。

文本语法：`AGG_loop("formulaId", iterator, start, end[, step])`

示例：
- `sum_loop("body-id", t, 1, n)` — 等价于 ∑(body(t), t=1..n)
- `product_loop("body-id", t, 1, n)` — 等价于 ∏(body(t), t=1..n)
- `avg_loop("body-id", t, 1, 100, 2)` — step=2

## 涉及文件

**前端：**
- `frontend/src/utils/graphText.ts` — renderNode 新增 loop case
- `frontend/src/components/editor/FormulaEditorPage.tsx` — 移除 loop 文本模式阻断

**后端：**
- `backend/internal/parser/serializer.go` — DAGToAST + ASTToDAG 新增 loop 支持
- `backend/internal/parser/validator.go` — 新增 NodeLoop case（当前 default 会报 unknown）

**测试：**
- `backend/internal/parser/serializer_test.go` 或 `roundtrip_test.go` — loop 序列化往返测试

## TODO

### 前端
- [x] `graphText.ts`: renderNode 新增 `loop` case，输出 `AGG_loop("formulaId", iter, start, end[, step])`
- [x] `FormulaEditorPage.tsx`: 移除 handleSetEditorMode 中的 loop 阻断逻辑

### 后端
- [x] `serializer.go` DAGToAST: NodeLoop → FunctionCall("AGG_loop", ...)
- [x] `serializer.go` ASTToDAG: 识别 `*_loop` 函数名 → NodeLoop（含 parseLoopFuncName 辅助函数）
- [x] `serializer.go` ASTToText: 非标识符变量名自动加引号（修复 UUID/含连字符 ID 的往返问题）
- [x] `validator.go`: 新增 `case domain.NodeLoop` 避免 "unknown node type" 错误

### 测试
- [x] 后端：5 个 loop 文本往返测试全部通过（含 step、表达式参数、DAG config 验证）
- [x] 全量后端测试通过（engine + parser）
- [x] 前端 build 无 TS 错误，113 测试全部通过
