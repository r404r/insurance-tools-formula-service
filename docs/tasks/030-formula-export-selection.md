# Task #030: 公式导出支持选择对象

## Status: done

## 需求

批量导出目前仅有"全量导出"，用户希望能选择部分公式导出。

## 设计

- 表格添加 checkbox 列（每行 + 表头全选本页）
- `selectedIds: Set<string>` 跨分页保留选择
- 工具栏条件显示 **Export Selected (N)** + **Clear Selection** 按钮
- 保留 Export All 作为一键导出全部的快捷方式
- 删除公式时自动从 selection 中移除

## 改动文件

- `frontend/src/components/shared/FormulaList.tsx`
  - selectedIds 状态 + 切换/全选/清空函数
  - exportSelectedMutation
  - 表格 checkbox 列
  - 工具栏条件按钮
  - deleteMutation onSuccess 清理 selection
- i18n (en/zh/ja): exportSelected, selectAllOnPage, selectRow, clearSelection

## 后端

**无变化**，复用 #029 的 `POST /formulas/export` 端点。
