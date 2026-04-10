# Task #027: 调色板颜色与节点颜色同步

## Status: done

## 需求

图形公式编辑界面最左侧的节点调色板中，每个节点类型的图标颜色应与实际画布上该节点的颜色一致，帮助用户快速识别。

## 改动

`frontend/src/components/editor/NodePalette.tsx`:
- 导入 `NODE_COLORS` 从 `nodePresentation.ts`
- 每个调色板项使用对应节点类型的 bg/border 颜色
- 左侧加 4px 彩色 border 作为视觉锚点
- 图标使用节点的 bg 作背景、border 作文字和边框色

## 效果

调色板 9 种节点类型现在显示 9 种不同颜色，与画布上实际节点完全一致。
