# Task #046: TableAggregate 文本模式守卫（task #040 P2 follow-up）

## Status: done

## 需求

Task #045 创建的「日本損害保険 チェインラダー LDF」seed 公式使用了 `NodeTableAggregate` 节点。打开此公式 → 切换到 Text Editor → 看到的是

```
// Unsupported node type tableAggregate
```

LaTeX 预览同时显示「Unable to render formula」。

根因是 task #040 引入 `NodeTableAggregate` 时只覆盖了引擎和验证器路径，**文本模式的两个序列化点（前后端）都没有同步**。Task #040 round 2 已经为 composite Conditional 修过同样的问题，本 task 把同样的模式应用到 TableAggregate。

## 设计

修复模式与 task #040 round 2 完全一致：

1. **后端** `backend/internal/parser/serializer.go`
   - `dagToASTWalk` 的 type-switch 加 `case domain.NodeTableAggregate`
   - 返回明确的错误："tableAggregate cannot be represented in text editor mode; please use the visual editor"
   - 前端可通过 substring 匹配 `"tableAggregate"` + `"visual editor"` 检测此错误

2. **前端** `frontend/src/utils/graphText.ts`
   - `reactFlowToText` 的 type-switch 加 `case 'tableAggregate'`
   - 抛 Error，措辞与后端对齐
   - `FormulaEditorPage.tsx` 现有的 try/catch 已经把错误塞进 textarea 作为注释 — 不需要改 UI

3. **i18n** `frontend/src/i18n/locales/{en,zh,ja}.json`
   - 加 `editor.tableAggregateNoTextMode` keys（与 `loopNoTextMode` 同一类）

4. **测试**
   - `backend/internal/parser/conditional_validator_test.go`（已有，task #039 round 2 添加的）— 加一个 `TestDAGToAST_RejectsTableAggregateWithClearMessage` 用例

5. **文档** `README.md` § Known Limitations + 三语 `formula-editor-guide-{en,zh,ja}.md` §6.6
   - 在 Loop / Composite Conditional 旁边加 TableAggregate

## 涉及文件

- `backend/internal/parser/serializer.go` — 加 case
- `backend/internal/parser/conditional_validator_test.go` — 加测试（这个文件已经覆盖类似的"text mode rejection"测试）
- `frontend/src/utils/graphText.ts` — 加 case
- `frontend/src/i18n/locales/{en,zh,ja}.json` — 加 key
- `README.md` — Known Limitations 加一段
- `docs/guide/formula-editor-guide-{en,zh,ja}.md` — §6.6 加一行

## TODO

- [x] 加 `case domain.NodeTableAggregate` 到 `dagToASTWalk`
- [x] 加 case 到 `reactFlowToText`
- [x] 加 i18n key (en/zh/ja)
- [x] 加单元测试 `TestDAGToAST_RejectsTableAggregateWithClearMessage`
- [x] 更新 README Known Limitations
- [x] 更新 三语公式 Guide §6.6
- [x] `go vet ./... && go test ./... -race && npm run build` 通过
- [x] 浏览器冒烟：打开 LDF → 切到 text mode → 看到友好提示而不是 raw error
- [x] codex review LGTM
- [x] commit

## 完成标准

- [x] LDF 公式切到 text mode 不再显示 raw "Unsupported node type" 错误
- [x] 后端 dagToASTWalk 对 tableAggregate 返回的错误包含 "tableAggregate" + "visual editor"
- [x] race detector + build 全绿
- [x] codex review LGTM
