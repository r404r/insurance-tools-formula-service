# Task #045: 日本保険公式 17 个内置公式 + 模版

## Status: done

## 需求

把 [`docs/specs/002-Japan-insurance-coverage-analysis.md`](../specs/002-Japan-insurance-coverage-analysis.md) 调研的 20 条日本保险公式中的 18 条（除 #14 完全信頼度 和 #19 異常危險準備金）作为：

1. **内置公式**（seed）：服务首次启动时自动创建在 DB 里
2. **计算式模版**（template）：在模版画廊里可见，用户可一键复制

公式 #2 `定期保険一時払純保険料` 已经在 seed 里（见 main.go 的 `seedFormula("定期保険一時払純保険料"...)`），不重复创建。所以新增 **17 个 seed + 17 个 template**。

## 设计

### 命名约定

| 分类 | 前缀 | spec # |
|---|---|---|
| 生命保険数理 | `日本生命保険 ...` | 1, 3, 4, 5 |
| 損害保険数理 | `日本損害保険 ...` | 6, 7, 8, 9, 10, 11, 12 |
| 信用度理論 | `日本損害保険 ...` | 13 |
| 損害賠償 | `日本損害賠償 ...` | 15, 16 |
| 経営健全性 | `日本生命保険 ...` | 17, 18 |
| 自賠責 | `日本自賠責 ...` | 20 |

Template ID 命名：`tpl-jp-NN-name`，NN 是 spec 编号便于追踪。

### 17 个公式逐条设计

| spec | 名称 | 公式 | 输入 | 复杂度 |
|---|---|---|---|---|
| 1 | 死亡率 | `q = d / l` | `d`, `l` | 极简：1 division |
| 3 | 基数 D_x | `D = v^x · l` | `v`, `x`, `l` | 简单：power + multiply |
| 4 | 将来法責任準備金 | `V = A − P · ä` | `A`, `P`, `a` | 简单：multiply + subtract |
| 5 | チルメル式責任準備金 | `Vz = V − α · (1 − a_part/a_full)` | `V`, `alpha`, `a_part`, `a_full` | 中：5 ops |
| 6 | 損害率 | `LR = (paid + adj) / premium` | `paid`, `adj`, `premium` | 简单：add + divide |
| 7 | 発生保険金 | `incurred = paid + (end_res − begin_res)` | `paid`, `end_res`, `begin_res` | 简单 |
| 8 | チェインラダー LDF | `LDF[j] = avg(dev_ratio WHERE dev_year=j)` | `dev_year` (动态), 表 `claims_triangle_sample` | TableAggregate |
| 9 | BF 法 | `ult = C + E · (1 − 1/f)` | `C`, `E`, `f` | 简单 |
| 10 | IBNR 要積立額 b | `b = (y1+y2+y3)/3/12` | `y1`, `y2`, `y3` | 简单 |
| 11 | 1/24 法未経過保険料 | `unearned = premium · (2·(13−start_month)−1)/24` | `premium`, `start_month` | 简单 |
| 12 | 短期料率返還 | `refund = premium · (1 − short_rate)` | `premium`, `short_rate` | 简单 |
| 13 | Bühlmann | `μ̂ = (n/(n+K))·X + (K/(n+K))·μ` | `X`, `mu`, `n`, `K` | 中：6 ops |
| 15 | 休業損害（自賠責） | `loss = 6100 · days` | `days` | 极简 |
| 16 | 逸失利益 | `lost = income · (1−rate) · leibniz` | `income`, `rate`, `leibniz` | 简单 |
| 17 | SMR | `SMR = SMM / (0.5·sqrt(R1²+(R2+R3)²+R4²)) · 100` | `SMM`, `R1`, `R2`, `R3`, `R4` | 中：sqrt + 多 op |
| 18 | 逆ざや | `negspread = (planned − actual) · reserve` | `planned`, `actual`, `reserve` | 简单 |
| 20 | 自賠責収支調整 | `surplus = premium − paid − admin` | `premium`, `paid`, `admin` | 简单 |

### 设计决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| #4 / #5 是否用 sub-formula 引用现有 seed？ | 不用，take A/ä as scalar inputs | 保持 template 自包含；用户可以手动接 sub-formula |
| #8 是否需要 seed 一个示例表？ | 是，新增 `claims_triangle_sample` | 否则 seed formula 加载即报错 |
| #11 是否用 lookup table？ | 不用，闭式公式 | 避免新增表 |
| 命名是否带 spec # 前缀？ | 不带（用日文公式名） | 公式名本身已经唯一 |
| 是否提取 graph helper 函数？ | 是，新增 `nVar / nConst / nOp / nFn / eEdge` | 17 个公式需要大量重复 boilerplate |

### 涉及文件

- `backend/cmd/server/main.go`:
  - 新增 graph 构造 helper 函数
  - 新增 17 个 `seedFormula(...)` 调用（按命名分组）
  - 新增 1 个 `seedTable(...)` 调用（claims_triangle_sample）
  - 更新 `seedFormulaNames[]` 加 17 条
  - 更新 `seedTableNames[]` 加 1 条
- `backend/internal/api/template_handler.go`:
  - 新增 17 个 `FormulaTemplate{...}` 条目到 `allTemplates`
  - 复用现有的 `tplVar / tplConst / tplOp / tplFn` 辅助函数

不动前端（template gallery 自动从 API 拉取）。

## TODO

- [x] 加 graph helper 到 main.go (`nVar / nConst / nOp / nFn / nTableAgg / eEdge`)
- [x] 加 claims_triangle_sample 表 seed
- [x] 加 17 个 seed formulas 到 main.go
- [x] 更新 seedFormulaNames + seedTableNames
- [x] 加 17 个 templates 到 template_handler.go (新 helpers `tplFnPlain` + `tplTableAgg`)
- [x] `go vet ./... && go test ./... -race && go build ./...` 通过
- [x] 重启 dev server (docker postgres profile)，验证 17 个 seeds 全部成功创建
- [x] curl `GET /api/v1/templates` 验证 17 个新模版返回（total 26）
- [x] 8 个代表性公式做 end-to-end 计算验证：#1, #3, #6, #8, #9, #13, #15, #17 全部正确
- [x] codex review 两轮 → P1（1/24 法符号反了）修复 → LGTM
- [x] 移到 backlog 已完成

## 完成标准

- [x] 服务首次启动后能在 `/api/v1/formulas` 列表里看到 17 个新公式
- [x] 模版画廊能看到 17 个新模版
- [x] race detector + build 全绿
- [x] codex review LGTM

## Codex review fixes

**Round 1 — P1**：「日本損害保険 1/24法未経過保険料」公式弄反了。原 closed-form 给的是**已経過**比率而不是 spec 要求的**未経過**比率。1月始期返回 23/24 而不是 1/24。修复方法：把 `(2·(13−start_month)−1)/24` 改成 `(2·start_month − 1)/24`，移除冗余的 c_13 + op_13m 节点，restructure graph。end-to-end 验证：

- start_month=1 premium=24000 → unearned=1000 (= 24000·1/24) ✓
- start_month=12 premium=24000 → unearned=23000 (= 24000·23/24) ✓

**Round 2 — LGTM**。Codex 注意到 start_month 没有边界校验（用户可传 1..12 之外的值），但这是 pre-existing 行为，不是本 task 引入的。
