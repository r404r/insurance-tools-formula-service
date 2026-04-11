# Backlog

## 待规划

### 🥇 引擎能力扩展（来自日本保险公式覆盖度调研，[specs/002-Japan-insurance-coverage-analysis.md](specs/002-Japan-insurance-coverage-analysis.md)）

- [ ] **优先级 2：表聚合节点（Table Aggregate Node）** — 详见 [specs/004-table-aggregate-node.md](specs/004-table-aggregate-node.md)。预估 3 天。解锁链梯法（公式 8），是当前唯一卡脖子的扩展点。

### 🔬 未来研究项目（来自 task #039 已知限制 + 引擎能力盘点）

> 这些项目都不阻塞当前工作，记录在此供将来评估。每条都已经在
> README.md § Known Limitations 中向用户公开。

- [ ] **文本模式支持复合 Conditional**：扩展 `parser/lexer.go` + `parser.go` 加入 `and` / `or` / `not` 关键字，并更新 `serializer.go` 的 `dagToASTWalk` Conditional 分支以发出复合条件 AST。完成后 task #039 引入的 composite conditional 才能在文本模式编辑。预估 2-3 天。
- [ ] **可视化编辑器支持复合 Conditional UI**：在 `NodePropertiesPanel` 加 "Add Condition" 按钮、combinator 切换、Negate 复选框，并自动管理 `condition_i` / `conditionRight_i` 端口名分配。当前只能通过手写 JSON / API 使用。预估 2 天。
- [ ] **混合 AND/OR 在单 Conditional 内**：当前一个 Conditional 节点的 combinator 是 uniform 的，需要嵌套两层来表达 `A AND (B OR C)`。如果实务中经常出现混合规则，可以引入 boolean 表达式节点（`logic_and` / `logic_or` / `logic_not`）让 Conditional 的 if 输入接 boolean 节点。预估 3-5 天。
- [ ] **统计分布函数**（`normal_cdf` / `normal_quantile` / `chi²` 等）：用 Abramowitz-Stegun 近似公式实现 normal 系列即可。解锁信用度理论的 `k` 分位数（公式 14）+ 未来 VaR/TVaR 风险测度。预估 1 天。
- [ ] **日期/时间算术**：原生 `date` 类型 + 日数计算 + 月份按比例。当前 1/24 法、短期料率返还都靠预计算表绕。预估 3 天。
- [ ] **第一公民的嵌套 Loop**：允许 Loop body 直接包含另一个 Loop 节点，不必经过 sub-formula。表达力不变，但消除人为切分。预估 2 天。
- [ ] **跨 Calculate 调用的状态持久化**：让连续多次 Calculate 共享一个"会话"，避免客户端 orchestrate 准备金/链梯滚动。预估 5 天，需要存储层和 API 改造，影响面较大。

### 其它

- [ ] Lookup Tables目前已经有基础的功能，请改造或者增加一种新功能，即能管理多种表，且能自定义每个表的表结构（比如有a，b，c，d四个字段，实际使用的时候通过a，b，c字段来定位d字段），并能批量通过csv或者Excel上传表的实际内容。
- [ ] E2E 测试
- [ ] 负载测试
- [ ] 更丰富的节点视觉效果

## 已排期

## 已完成

- [039-conditional-logical-operators.md](tasks/039-conditional-logical-operators.md) — Conditional 节点的 AND/OR/NOT 支持（spec 003 优先级 1）（2026-04-11）
- [038-japanese-navbar-fix.md](tasks/038-japanese-navbar-fix.md) — 日语 Navbar 换行修复（i18n 简洁化 + whitespace-nowrap）（2026-04-11）
- [037-table-data-cache.md](tasks/037-table-data-cache.md) — Lookup Table 数据缓存（性能报告方向 B）（2026-04-11）
- [036-batch-test-parallel.md](tasks/036-batch-test-parallel.md) — Batch Test 服务端并行化 + 总执行时间（2026-04-11）
- [035-editable-node-id.md](tasks/035-editable-node-id.md) — 可视化编辑器支持修改节点 ID + 合法/冲突校验（2026-04-11）
- [034-formula-canvas-stale-closure-fix.md](tasks/034-formula-canvas-stale-closure-fix.md) — 新拖入节点无法连线（stale closure）修复（2026-04-11）
- [033-term-life-batch-n1-100.md](tasks/033-term-life-batch-n1-100.md) — 定期保険纯保険料 批量测试数据 n=1..100（2026-04-11）
- [032-formula-list-density.md](tasks/032-formula-list-density.md) — 公式一览行信息密度优化（2026-04-11）
- [031-selection-bar-fix.md](tasks/031-selection-bar-fix.md) — 选择状态下顶栏布局修复（2026-04-11）
- [030-formula-export-selection.md](tasks/030-formula-export-selection.md) — 导出支持选择对象（2026-04-11）
- [029-formula-import-export.md](tasks/029-formula-import-export.md) — 公式导入导出（2026-04-11）
- [028-formula-copy.md](tasks/028-formula-copy.md) — 公式复制功能（2026-04-10）
- [027-palette-color-sync.md](tasks/027-palette-color-sync.md) — 调色板颜色与节点颜色同步（2026-04-10）
- [026-readme-update.md](tasks/026-readme-update.md) — README 全量更新 + 新截图（2026-04-10）
- [025-formula-delete-seed-reset.md](tasks/025-formula-delete-seed-reset.md) — 公式删除 + 预置数据重置（2026-04-10）
- [024-user-guide.md](tasks/024-user-guide.md) — 公式编辑器操作指南 中/英/日三语（2026-04-10）
- [023-life-insurance-loop-formulas.md](tasks/023-life-insurance-loop-formulas.md) — 生命保険数理 Loop 公式支持（2026-04-10）
- [022-node-description.md](tasks/022-node-description.md) — 节点 Description 功能（2026-04-10）
- [021-loop-text-mode.md](tasks/021-loop-text-mode.md) — Loop 节点文本模式支持（2026-04-10）
- [020-loop-node.md](tasks/020-loop-node.md) — Loop 节点实现（2026-04-09）
- [019-formula-graph-validation.md](tasks/019-formula-graph-validation.md) — 公式图结构合理性校验增强（2026-04-09）

- [001-user-management.md](tasks/001-user-management.md) — 用户管理实现（2026-04-06）
- [002-lookup-table-management.md](tasks/002-lookup-table-management.md) — Lookup Tables 管理实现（2026-04-06）
- [003-multi-key-lookup-table.md](tasks/003-multi-key-lookup-table.md) — Lookup Table 多 Key 支持（2026-04-06）
- [004-cache.md](tasks/004-cache.md) — 计算结果缓存 + 管理页面（2026-04-07）
- [005-batch-test.md](tasks/005-batch-test.md) — 大批量测试功能（2026-04-07）
- [006-concurrency-control.md](tasks/006-concurrency-control.md) — 并发控制与 DB 连接池（2026-04-07）
- [007-admin-settings.md](tasks/007-admin-settings.md) — 管理员系统设置页面（最大并发数）（2026-04-07）
- [008-version-diff.md](tasks/008-version-diff.md) — 版本 diff 视图（2026-04-07）
- [009-formula-templates.md](tasks/009-formula-templates.md) — 保险领域公式模板画廊（2026-04-07）
- [010-workflow-guard-hooks.md](tasks/010-workflow-guard-hooks.md) — 工作流守卫 Hook（2026-04-07）
- [011-merge-cache-into-settings.md](tasks/011-merge-cache-into-settings.md) — Cache 管理整合到 Settings 页面（2026-04-07）
- [012-formula-list-pagination.md](tasks/012-formula-list-pagination.md) — 公式一览分页（2026-04-07）
- [013-formula-description-edit.md](tasks/013-formula-description-edit.md) — 公式 Description 内联编辑（2026-04-08）
- [014-postgres-store.md](tasks/014-postgres-store.md) — PostgreSQL Store 实现（2026-04-08）
- [015-mysql-store.md](tasks/015-mysql-store.md) — MySQL Store 实现（2026-04-08）
- [016-custom-node-components.md](tasks/016-custom-node-components.md) — 自定义 React Flow 节点组件（2026-04-08）
- [017-latex-formula-input.md](tasks/017-latex-formula-input.md) — LaTeX 表达式输入创建公式（2026-04-08）
- [018-latex-test-suite.md](tasks/018-latex-test-suite.md) — LaTeX 公式输入功能测试套件（2026-04-09）
