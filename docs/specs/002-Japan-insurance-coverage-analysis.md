# 002 — 日本保险数理公式覆盖度分析

**关联文档**：[002-Japan-insurance-ref.md](./002-Japan-insurance-ref.md)（20 条核心日本保险数理公式说明）
**分析日期**：2026-04-11
**分析对象**：formula-service 当前实现 vs `002-Japan-insurance-ref.md` 中列出的 20 条公式
**结论摘要**：20 条中 17 条原生支持、2 条需要工程 workaround、1 条 UX 缺陷但语义正确，**没有完全不可表达的公式**。

---

## 1. 引擎当前能力盘点

基于 `backend/internal/engine/`、`backend/internal/domain/`、`backend/internal/parser/` 的代码扫描。

### 1.1 节点类型（9 种）

| NodeType | 用途 |
|---|---|
| `NodeVariable` | 命名输入变量 |
| `NodeConstant` | 字面常量 |
| `NodeOperator` | 二元算术：+, −, *, /, ^, mod |
| `NodeFunction` | 函数调用：sqrt / abs / ln / exp / round / floor / ceil / min / max |
| `NodeTableLookup` | 查找表（支持多键复合，由 `\|` 拼接） |
| `NodeConditional` | 单比较 if-then-else（eq/ne/gt/ge/lt/le） |
| `NodeAggregate` | 静态项集合的聚合（sum/avg/...） |
| `NodeSubFormula` | 按 ID 调用其它公式，引擎做循环检测 |
| `NodeLoop` | 整数边界 loop，支持 sum/product/count/avg/min/max/last/fold 八种聚合 |

### 1.2 数值精度
- `shopspring/decimal` 任意精度
- 中间精度可配（默认 16 位），输出精度可配（默认 8 位）
- 超越函数（sqrt/ln/exp）走 float64 中转，精度 ~15-17 有效位

### 1.3 关键能力总结
- **Loop**：支持 `sum / product / count / avg / min / max / last / fold` 8 种 aggregation；fold 模式有显式 accumulator 变量 + initValue
- **Sub-formula**：可按 ID 调用，跨调用 result memoization 已实现（task #037 之后还有 table parsed-rows cache）
- **Lookup tables**：多键复合支持，复合键以 `\|` 拼接
- **Conditional**：单比较 if-else，6 种比较运算符

### 1.4 已知缺失（按对 002 spec 的影响排序）
1. **Conditional 不支持逻辑组合**（无 AND/OR/NOT）
2. **没有"对表的某列做带条件 aggregate"**的原生节点
3. **Loop 不能直接嵌套 Loop**（必须经 sub-formula 间接嵌套）
4. **没有标准统计分布函数**（normal_cdf, normal_quantile, chi², ...）
5. **没有日期/时间算术**
6. **Trig 函数缺失**（sin/cos/tan）
7. **跨 `Calculate` 调用的状态持久化**（每次调用是无状态的）

---

## 2. 20 条公式的逐条映射

### 生命保险数理（公式 1-5）

| # | 公式 | 形式 | 支持等级 | 实现路径 |
|---|---|---|---|---|
| 1 | 死亡率 q_x = d_x/l_x | 标量除法 | ✅ 原生 | Lookup table 列 / 简单除法 |
| 2 | 一时払纯保险料 A¹_{x:n\|} = Σ vᵗ⁺¹·ₜpₓ·q_{x+t} | Loop sum | ✅ **已实现** | Loop+sum，task #033 同款 |
| 3 | 基数 D_x, N_x, C_x, M_x | 标量 + Loop sum 累积 | ✅ 原生 | Loop sum 起点变量化，或预计算列 |
| 4 | 将来法责任准备金 ₜV_x = A_{x+t} − Pₓ·ä_{x+t} | 子公式 + 算术 | ✅ 原生 | Sub-formula 调用 |
| 5 | チルメル式 ₜV_x^Z | 子公式 + 算术 | ✅ 原生 | 同上 |

**生命保险结论**：5/5 全部原生支持。task #023 / #033 已经验证了 loop+sub-formula+table 这条核心链路。

### 损害保险数理（公式 6-12）

| # | 公式 | 形式 | 支持等级 | 实现路径 |
|---|---|---|---|---|
| 6 | 损害率（W/P, E/I 两种） | 标量除法 | ✅ 原生 | 直接算术 |
| 7 | 发生保险金 = 支払 + ΔReserve | 标量算术 | ✅ 原生 | 直接算术 |
| **8** | **チェインラダー法 LDF** | **三角形矩阵迭代** | ⚠️ **可表达但 awkward** | 需要二维 lookup table + 嵌套 loop |
| 9 | BF 法 C + E·(1 − 1/f) | 标量算术 | ✅ 原生 | 直接算术 |
| 10 | 法定 IBNR 积立 a/b | 标量算术 + 平均 | ✅ 原生 | 直接算术 |
| 11 | 1/24 法未经过保险料 | 表查询 or 简单公式 | ✅ 原生 | 12 行表 / 闭式公式 |
| 12 | 短期料率返还 | 表查询 + 算术 | ✅ 原生 | 月份索引表 |

**损害保险结论**：6/7 原生支持。**链梯法（公式 8）是 20 条里最棘手的一个**——技术上可以通过 multi-key lookup table + sub-formula 嵌套 loop 实现，但流程繁琐、调试痛苦。

### 信用度理论（公式 13-14）

| # | 公式 | 形式 | 支持等级 | 实现路径 |
|---|---|---|---|---|
| 13 | Bühlmann μ̂ = Z·X̄ + (1−Z)·μ | 标量算术 | ✅ 原生 | 直接算术 |
| **14** | **完全信頼度 n\* = (k/P)²·Var/E²** | **算术 + 标准正态分位数 k** | ⚠️ **k 值需外部** | 缺 normal_quantile，硬编码 1.96/2.576 |

### 损害赔偿（公式 15-16）

| # | 公式 | 形式 | 支持等级 |
|---|---|---|---|
| 15 | 休业损害（自賠責 / 实损） | 标量算术 + 分支 | ✅ 原生 |
| 16 | 逸失利益 = 年収·(1−生活費控除率)·ライプニッツ係数 | 算术 + 年金现价 | ✅ 原生（年金现价用 Loop sum 或闭式） |

### 经营健全性（公式 17-18）

| # | 公式 | 形式 | 支持等级 |
|---|---|---|---|
| 17 | SMR = SMM / (½·√(R₁² + (R₂+R₃)² + R₄²)) × 100 | sqrt + 算术 | ✅ 原生（sqrt 已支持） |
| 18 | 逆ざや = (予定利率−運用利回り)·準備金 | 标量算术 | ✅ 原生 |

### 特殊保险数理（公式 19-20）

| # | 公式 | 形式 | 支持等级 | 备注 |
|---|---|---|---|---|
| 19 | 異常危險準備金 + 取崩判定 | 算术 + Conditional | ⚠️ **UX 缺陷** | 取崩条件常含 AND/OR，目前只能嵌套两层 Conditional |
| 20 | 自賠責 No-profit/No-loss 调整 | 算术（流程为主） | ✅ 原生 |

---

## 3. 三个真实薄弱点的详细分析

### 薄弱点 1 — 链梯法（公式 8）：唯一的硬伤

**为什么硬**：链梯法本质是事故年 × 经过年的稀疏二维矩阵 + 行内累积比 + 列向投影。

```
           Dev1    Dev2    Dev3    Dev4
AY 2023:   100  →  127  →  149  →  168 (final)
AY 2024:    95  →  121  →  144  →  ?
AY 2025:   105  →  133  →  ?    →  ?
AY 2026:    98  →  ?    →  ?    →  ?

LDF₁ = avg(127/100, 121/95, 133/105) ≈ 1.259
LDF₂ = avg(149/127, 144/121) ≈ 1.181
LDF₃ = avg(168/149) ≈ 1.127
```

**当前能不能做？技术上能，路径很弯**：
- 三角形数据塞进 multi-key lookup table，复合键 `(acc_year\|dev_year)` → cumulative claim
- 外层 Loop 遍历 dev_year 计算每列 LDF（loop body 必须抽成 sub-formula，因为不支持嵌套 Loop）
- sub-formula 内部用 fold 累加分子分母
- 投影阶段再来一组类似结构

**问题在 ergonomic，不在表达能力**：
1. 三角形数据本质二维，但只能用字符串拼接的 composite key 模拟
2. 嵌套 loop 必须拆成至少 2 个公式，调试时跨文件跳来跳去
3. "忽略空 cell"的 filter 语义只能在 loop body 里手写 conditional

### 薄弱点 2 — 标准正态分位数（公式 14）

`k` 是标准正态分布的分位数（95% 信頼区間 → k = 1.96）。当前引擎没有 `normal_quantile(p)`，使用者只能：
- (a) 把 k 当作外部输入传进来
- (b) 在公式里硬编码 1.96 / 2.576 / 1.645 等几个常用值为常量

实务上不算大问题（k 几乎只用 95% / 99% / 99.5% 这几个固定值），但**未来做 VaR/TVaR/巨灾 fitting 时会成为硬墙**。

### 薄弱点 3 — Conditional 缺逻辑组合（影响公式 19、其它）

公式 19 异常危险准备金的取崩条件常见是：

> **if** loss_ratio > 0.5 **AND** cumulative_release < release_cap **then** ...

当前 Conditional 节点只支持单个比较，做复合条件只能嵌套两层 Conditional。这不影响数学正确性，但**做风险阈值规则时 UX 很差**：
- 公式图变得"三角形结构"，可读性下降
- 文本编辑器里要写 `if A then if B then X else Y else Y`，反人类

---

## 4. 推荐扩展（按 ROI 排序）

### 🥇 优先级 1 — Conditional 加 AND / OR / NOT

**收益**：公式 19 的 UX 缺陷消失，未来所有"if A and B then..."业务规则可读性提升一个数量级
**工作量**：~1 天
**详细设计**：见 [`003-conditional-logical-operators.md`](./003-conditional-logical-operators.md)

### 🥈 优先级 2 — 表的 SQL 风格 Aggregate 节点（filter + sum/avg/count）

**收益**：链梯法（公式 8）从噩梦变成两行公式；任何"对历史数据做带条件聚合"的场景一并解锁
**工作量**：~3 天
**详细设计**：见 [`004-table-aggregate-node.md`](./004-table-aggregate-node.md)

### 🥉 优先级 3 — 第一公民的嵌套 Loop

允许 Loop body 直接包含另一个 Loop 节点，不必经过 sub-formula。表达能力等价，但消除人为切分。
**工作量**：~2 天

### 第二梯队（可选）

| 扩展 | 工作量 | 影响公式 | 备注 |
|---|---|---|---|
| `normal_cdf` / `normal_quantile` 函数 | ~0.5 天 | 14 | Abramowitz-Stegun 近似即可 |
| Date arithmetic | ~3 天 | 11, 12 | 当前用预计算表能绕，性价比一般 |
| Matrix 类型 | 数周 | 未来 SMR 升级 | 工程量过大，等真有需求再做 |

---

## 5. 结论

| 指标 | 值 |
|---|---|
| **20 条中纯原生支持** | 17 |
| **20 条中需要 workaround** | 2（公式 8、14） |
| **20 条中 UX 缺陷但语义正确** | 1（公式 19） |
| **20 条中完全不可实现** | 0 |

**当前 formula-service 已经是一个可以承接日本生命+损害保险数理 85% 实务公式的合格引擎**——尤其在生命保险（公式 1-5）和大部分损害保险算术（6-7、9-13、15-20）上没有任何短板。

**唯一真正卡脖子的扩展点是链梯法 / 三角形矩阵聚合**（公式 8）。如果实际工作中 IBNR / 准备金链梯法是高频场景，**优先级 2 的"Table aggregate 节点"是最值得做的扩展**——它把唯一的硬伤直接消掉，工程量也不大。

---

## 引用

- 引擎源码：`backend/internal/engine/`、`backend/internal/domain/formula.go`
- 已验证的案例：task #023（生命保险 Loop 公式）、task #033（一时払纯保険料 batch test）
- 性能基线：[../performance/001-batch-test-speedup-analysis.md](../performance/001-batch-test-speedup-analysis.md)
