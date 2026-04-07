# Task #008: 版本 Diff 视图

## Status: done

## 需求

在版本列表页面，允许用户查看两个版本之间的差异：
- 每个版本行（v2 起）显示「与上一版本对比」按钮
- 点击后弹出 Diff 面板，列出节点/边的新增、删除、修改
- 修改节点展示 Before / After 配置对比

## 设计

### 后端（已完成）

`GET /api/v1/formulas/{id}/diff?from=X&to=Y`
返回 `VersionDiff`：
```json
{
  "fromVersion": 1, "toVersion": 2,
  "addedNodes": [...], "removedNodes": [...],
  "modifiedNodes": [{"nodeId":"n1","before":{...},"after":{...}}],
  "addedEdges": [...], "removedEdges": [...]
}
```

### 前端

- `src/types/formula.ts` — 追加 `VersionDiff`, `NodeDiff` 类型
- `src/api/versions.ts`（新） — `getVersionDiff(formulaId, from, to)`
- `src/components/version/VersionDiffModal.tsx`（新） — 弹窗展示 diff
  - Header：v{from} → v{to}，变更摘要（+N / -N / ~N nodes, +N / -N edges）
  - 三个区块：新增节点（绿）、删除节点（红）、修改节点（黄，Before/After 配置对比）
  - 边变更区块：新增边（绿）、删除边（红）
- `src/components/version/VersionsPage.tsx` — 每行（v2 起）追加「对比」按钮，点击触发弹窗
- i18n：zh/en/ja 追加 `diff.*` keys

## 涉及文件

- `frontend/src/types/formula.ts`
- `frontend/src/api/versions.ts`（新）
- `frontend/src/components/version/VersionDiffModal.tsx`（新）
- `frontend/src/components/version/VersionsPage.tsx`
- `frontend/src/i18n/locales/zh.json`, `en.json`, `ja.json`

## TODO

- [x] 创建任务文件
- [x] types/formula.ts：添加 VersionDiff / NodeDiff 类型
- [x] api/versions.ts：getVersionDiff()
- [x] VersionDiffModal.tsx：diff 展示弹窗
- [x] VersionsPage.tsx：添加「对比」按钮
- [x] i18n（zh/en/ja）
- [x] codex review + fix P1/P2
- [x] 提交

## 完成标准

- [x] 功能正常
- [x] 测试通过
- [x] commit + codex review
