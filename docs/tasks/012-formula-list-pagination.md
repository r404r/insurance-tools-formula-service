# Task #012: 公式一览页面分页功能

## Status: in-progress

## 需求

公式列表当前一次性加载最多 50 条（后端默认 limit），不支持翻页。
需要在前端追加分页 UI，允许用户浏览超过一页的公式列表。

## 设计

### 后端
无需改动。`GET /api/v1/formulas` 已支持 `limit`/`offset`，返回 `total`。

### 前端

- `FormulaList.tsx`
  - 新增 `page` state（default 1），`PAGE_SIZE = 20`
  - 每次 query 带 `limit=20&offset=(page-1)*20`
  - 修复：当前 API 调用使用 `page`/`pageSize` 参数名，但后端期望 `limit`/`offset`，需对齐
  - 同时读取响应中的 `total`（当前被丢弃）
  - 搜索/domain 切换时重置 page=1
  - 底部添加分页栏：「上一页 / 页码 / 下一页」+ 总条数提示

### 页码 UI 设计

```
共 N 条  <  1  2  3 … 10  >
```
- 当前页高亮
- 首页/末页始终显示；中间最多显示 5 个相邻页码，超出用「…」省略
- 仅 1 页时不显示分页栏

## 涉及文件

- `frontend/src/components/shared/FormulaList.tsx`

## TODO

- [x] 创建任务文件
- [ ] FormulaList.tsx：page state + API limit/offset 对齐
- [ ] FormulaList.tsx：搜索/domain 变化时 reset page
- [ ] FormulaList.tsx：分页栏 UI
- [ ] tsc --noEmit 通过
- [ ] codex review + fix P1/P2
- [ ] 提交

## 完成标准

- [ ] 分页功能正常
- [ ] 测试通过
- [ ] commit + codex review
