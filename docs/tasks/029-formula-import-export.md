# Task #029: 公式导入导出

## Status: done

## 需求

- 单个公式导出（JSON）
- 批量公式导出（当前过滤结果）
- 批量导入

## 设计

### 导出格式

```json
{
  "version": "1.0",
  "exportedAt": "2026-04-10T...",
  "formulas": [
    {
      "sourceId": "原UUID",
      "sourceVersion": 1,
      "name": "...",
      "domain": "life",
      "description": "...",
      "graph": { ... }
    }
  ]
}
```

### 设计决策

- **版本**: 仅导出最新版本（版本历史是系统内部概念）
- **引用**: subFormula/tableLookup 的引用 ID 保持不变（用户负责跨系统依赖）
- **ID**: 导入时生成新 UUID，旧 ID 保留为 `sourceId`（信息性）
- **状态**: 导入的公式 v1 = `draft`
- **错误处理**: Best-effort，单条失败不阻塞整批，返回汇总

### 后端 API

| 方法 | 路径 | 权限 |
|------|------|------|
| POST `/formulas/export` | PermFormulaView | body `{"ids":[...]}` |
| POST `/formulas/import` | PermFormulaCreate | body: 导出的 JSON |

### 前端 UI

- 公式列表每行：Export 按钮（旁边 Copy / Delete）
- 工具栏：**Import** + **Export All** 按钮
- 导入成功后显示汇总 dialog

## TODO

- [x] 后端：export handler + import handler + routes
- [x] 前端 API client: exportFormulas, importFormulas
- [x] 前端 UI: Export 行按钮 + Import 工具栏按钮 + Export All 工具栏按钮
- [x] i18n (en/zh/ja)
- [x] codex review — 7 issues found, all fixed
- [x] 全量测试通过 (backend + 118 frontend)

## Codex Review 修复摘要

1. ✅ **Import 不校验图结构** → 调用 `parser.ValidateGraph`，拒绝环、断边、重复节点、无输出、无效配置
2. ✅ **Rollback 错误被吞掉** → 匹配 copy 的模式，rollback 失败时附加到错误消息
3. ✅ **Export All 被 limit=200 硬顶截断** → 后端 cap 从 200 提升至 500
4. ✅ **Export 静默跳过缺失公式** → 添加 `X-Export-Requested`/`X-Export-Exported` 响应头，前端检测并弹 partial 警告
5. ✅ **Export 路由未显式要求权限** → 添加 `PermFormulaView` middleware
6. ✅ **前端文件名消毒不完整** → 新 `sanitizeFilename` 过滤控制字符 + 前导点
7. ✅ **import 失败回退英文** → `importParseError` key 三语本地化
