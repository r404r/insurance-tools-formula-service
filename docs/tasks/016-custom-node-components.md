# Task #016: 自定义 React Flow 节点组件

## Status: done

## 需求

目前所有节点使用同一个 `FormulaNode` 组件，仅颜色不同，外形完全相同。
需要为每种节点类型创建视觉上有差异的专属组件，提升可读性和视觉体验。

## 设计

### 各节点视觉设计

| 类型 | 外形 | 特色 |
|------|------|------|
| variable | 左侧蓝色粗边框强调 | 上方数据类型徽章（num/str/bool），变量名加粗 |
| constant | 普通矩形 | 等宽字体大号显示数值，amber 色 |
| operator | 圆形 | 超大运算符符号（+ − × ÷ ^ %），pink 色 |
| function | 普通矩形 | `fn()` 等宽字体格式，green 色 |
| subFormula | 普通矩形 | "sub-formula" 标签 + 公式名斜体，indigo 色 |
| tableLookup | 普通矩形 | ▤ 图标 + `.列名`，purple 色 |
| conditional | 普通矩形（较高） | if + 大号比较符号（> < ≥ ≤ = ≠），orange 色 |
| aggregate | 普通矩形 | Σ 符号 + 聚合函数名，teal 色 |

### 实现策略

- 保持 React Flow 节点类型为 `formulaNode`（向后兼容，不需要改序列化格式）
- 新建 `nodeVariants.tsx`：每种类型的内容组件
- 修改 `FormulaNode.tsx`：根据 `nodeType` 派发到对应内容组件，operator 用圆形容器
- 修改 `nodePresentation.ts`：更新 `estimateNodeSize`（operator: 72×72，conditional: 更高）

### 涉及文件

- `frontend/src/components/editor/nodeVariants.tsx`（新建）
- `frontend/src/components/editor/FormulaNode.tsx`（修改）
- `frontend/src/components/editor/nodePresentation.ts`（修改 estimateNodeSize）

## TODO

- [x] 创建任务文件
- [x] nodeVariants.tsx：8 种内容组件
- [x] FormulaNode.tsx：派发逻辑 + operator 圆形容器
- [x] nodePresentation.ts：更新 estimateNodeSize
- [x] tsc --noEmit 通过
- [x] codex review + fix P1/P2
- [x] 提交

## Codex Review

- P2 fix: 修正 COMPARATOR_SYMBOLS 键名（gte→ge, lte→le, neq→ne）以匹配实际持久化值
- P2 fix: 恢复 operator 节点的 L/R/Out 端口标签，避免用户混淆非对称运算符方向

## 完成标准

- [x] 各节点外观有明显视觉差异
- [x] tsc 通过
- [x] commit
