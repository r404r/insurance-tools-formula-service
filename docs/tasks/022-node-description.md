# Task #022: 节点 Description 功能

## Status: done

## 需求

在公式图形编辑界面中，给每个节点追加 description（描述）字段。用户可在属性面板中编辑，节点悬浮时显示 tooltip。

## 设计

- `FormulaNode` 新增 `Description string` 字段（backend）/ `description?: string`（frontend）
- 属性面板顶部统一展示 description textarea（所有节点类型共用）
- 节点视觉：hover 时显示 title tooltip
- 不影响计算引擎、文本模式、图校验

## TODO

- [x] `domain/formula.go`: FormulaNode 新增 `Description string` 字段（omitempty）
- [x] `types/formula.ts`: FormulaNode 新增 `description?: string`
- [x] `nodePresentation.ts`: FormulaNodeData 新增 description，createNodeData 接受第三参数
- [x] `graphSerializer.ts`: apiToReactFlow / reactFlowToApi 双向处理 description
- [x] `FormulaNode.tsx`: 添加 title tooltip（hover 显示 description）
- [x] `NodePropertiesPanel.tsx`: 添加 description textarea（所有节点类型共用）
- [x] `FormulaCanvas.tsx`: createNodeData 第三参数默认空，无需修改
- [x] `FormulaEditorPage.tsx`: enrichSubFormulaNodes 和 handleNodeDataChange 保留 description
- [x] i18n: 复用已有 `formula.description` key
- [x] 全量测试通过（backend + frontend 118 tests）
