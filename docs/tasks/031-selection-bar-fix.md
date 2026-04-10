# Task #031: 选择状态下顶栏布局修复

## Status: done

## 问题

选中公式后，顶部主工具栏的按钮从 5 个变成 7 个，超出 `max-w-6xl` 容器宽度，导致：
- 标题 "Formula List" 挤压成两行
- 每个按钮内部文字也强制换行
- 视觉非常凌乱

## 方案

将选择相关按钮（Clear + Export Selected）从顶部主工具栏中移出，作为**独立的选择操作条**显示在 filter 和 table 之间，仅在 `selectedIds.size > 0` 时出现。

布局：
```
[顶栏：5 buttons 不变]
[搜索框]
[Category tabs]
[选择操作条：N selected | Clear | Export Selected (N)]  ← 条件显示
[表格]
```

## 改动

- `FormulaList.tsx`: 从顶栏移除选择按钮，新增独立 selection bar div
- i18n: 新增 `formula.selectionCount` 三语
