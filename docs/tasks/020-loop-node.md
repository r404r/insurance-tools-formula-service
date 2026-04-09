# Task #020: Loop 节点实现

## Status: done

## 需求

按照 `docs/specs/001-loop-node-spec.md` 实现 Loop 节点，并在开始实现之前对规格做以下细化补充：

1. 新增 `NodeLoop` 节点类型和 `LoopConfig` domain 结构
2. 后端引擎支持 Loop 节点执行（类似 subFormula 特殊路径）
3. 后端验证器支持 Loop 节点配置和端口校验
4. 前端类型、节点调色板、节点视觉、属性面板全部支持 Loop
5. 图校验扩展 Loop 端口规则
6. 文本模式切换时屏蔽含 Loop 的图
7. 补充 i18n、后端/前端单元测试

## 规格补充说明

规格本身完整，以下是实现层面的细化：

### 迭代序列生成（精确算法）

```
step > 0:  i = start; i <= end (inclusiveEnd=true) 或 i < end (inclusiveEnd=false); i += step
step < 0:  i = start; i >= end (inclusiveEnd=true) 或 i > end (inclusiveEnd=false); i += step
step = 0:  运行时立即报错
step 未连接端口时：默认使用 1
```

若 `step > 0` 但 `start > end`，或 `step < 0` 但 `start < end`：迭代次数为 0，报错。

所有边界值均需是整数值（`d.Equal(d.Truncate(0))` 为 true 才合法）。

### LoopRunner 类型

与 `SubFormulaRunner` 对称，新增：

```go
type LoopRunner func(ctx context.Context, node *domain.FormulaNode,
    nodeInputs map[string]Decimal, seedInputs map[string]Decimal) (Decimal, error)
```

`Executor` 持有 `loopRunner LoopRunner`，在 `evaluateAndStore` 中对 `NodeLoop` 走此路径。

### EngineConfig 扩展

`EngineConfig` 新增 `MaxLoopIterations int`（默认 1000）。

`executeLoop` 优先取 `min(config.maxIterations, engine.config.MaxLoopIterations)`。

### 子公式调用与 call stack 保护

Loop 调用子公式时复用 `withSubFormulaCall(ctx, cfg.FormulaID, version.Version)`，
保证 Loop body 中如果再引用父公式会被拒绝。

### seedInputs 语义

Loop 每轮执行时：
- `childInputs = cloneDecimalMap(seedInputs)` （继承父计算上下文的所有变量输入）
- 注入 `childInputs[cfg.Iterator] = currentIterValue`
- 不把 `nodeInputs["start"]/"end"/"step"` 传入子公式（Loop-specific）

## 涉及文件

**后端：**
- `backend/internal/domain/formula.go`
- `backend/internal/engine/engine.go`
- `backend/internal/engine/parallel.go`
- `backend/internal/engine/loop_test.go`（新建）

**前端：**
- `frontend/src/types/formula.ts`
- `frontend/src/components/editor/nodePresentation.ts`
- `frontend/src/components/editor/nodeVariants.tsx`
- `frontend/src/components/editor/NodePalette.tsx`（若存在）
- `frontend/src/components/editor/NodePropertiesPanel.tsx`
- `frontend/src/utils/graphValidation.ts`
- `frontend/src/components/editor/FormulaEditorPage.tsx`
- `frontend/src/i18n/locales/en.json`
- `frontend/src/i18n/locales/zh.json`
- `frontend/src/i18n/locales/ja.json`

## TODO

### 后端
- [x] 创建 task 文件
- [ ] `domain/formula.go`: 新增 `NodeLoop NodeType = "loop"` 和 `LoopConfig` struct
- [ ] `engine/engine.go`: 新增 `MaxLoopIterations` 到 `EngineConfig`，更新 `DefaultEngineConfig`，实现 `executeLoop`，更新 `validateNodeConfig` 和 `validateRequiredPorts`
- [ ] `engine/parallel.go`: 新增 `LoopRunner` 类型，`Executor` 持有 `loopRunner`，`NewExecutor` 接收参数，`evaluateAndStore` 分派 `NodeLoop`
- [ ] `engine/engine.go`: `NewEngine` 将 `executeLoop` 传给 `NewExecutor`
- [ ] `engine/loop_test.go`: 编写单元测试（见规格 §12.1）

### 前端
- [ ] `types/formula.ts`: `NodeType` 加 `loop`，新增 `LoopConfig` interface，`NodeConfig` 联合类型加入
- [ ] `nodePresentation.ts`: 加 loop 颜色，`getInputPorts` 加 loop 分支，`nodeLabel` 加 loop，`defaultNodeConfig` 加 loop
- [ ] `nodeVariants.tsx`: 新增 `LoopInner` 组件
- [ ] `FormulaNode.tsx` / `nodeVariants.tsx` 接入 `LoopInner` 渲染
- [ ] `NodePalette.tsx`: 加入 loop 节点
- [ ] `NodePropertiesPanel.tsx`: 加入 loop 属性编辑面板（Body Formula / Iterator / Aggregation / InclusiveEnd / MaxIterations / Version）
- [ ] `graphValidation.ts`: `validateGraph` 加入 loop 端口规则（start/end 必连，step 可选，aggregation 合法值）
- [ ] `FormulaEditorPage.tsx`: 文本模式切换时检测 loop 节点，若存在则阻止并提示
- [ ] i18n: en/zh/ja 新增 loop 相关 key（loop, iterator, aggregation, inclusiveEnd, maxIterations, start, end, step, bodyFormula, loopNoTextMode）

### 验证
- [ ] 后端 `go test ./...` 通过
- [ ] 前端 `npm run build` 无 TS 错误
- [ ] 前端单元测试通过（graphValidation loop 规则）
- [ ] 手动测试：创建 Loop 节点，配置子公式，保存计算
- [ ] codex review + commit

## 完成标准

- [ ] Loop 节点可在可视化编辑器中创建、配置、连线
- [ ] 保存含 Loop 的公式，后端成功执行并返回正确结果
- [ ] 切换文本模式时提示不支持
- [ ] 环路检测、孤立节点检测均不受 Loop 节点影响（Loop 本身是有向边，不引入 DAG 环）
- [ ] 所有后端测试通过
