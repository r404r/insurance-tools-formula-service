# 004 — 表聚合节点（Table Aggregate Node）

**关联文档**：[002-Japan-insurance-coverage-analysis.md](./002-Japan-insurance-coverage-analysis.md) §4 优先级 2
**优先级**：🥈 第二高
**预估工作量**：3 天
**状态**：spec — 待实现
**前置依赖**：无（可与 003 并行）

---

## 1. 背景与动机

`002-Japan-insurance-coverage-analysis.md` §3 中详细论证了**链梯法（公式 8）是当前引擎唯一真正卡脖子的扩展点**。它的结构本质是：

```
对一张三角形数据表 (acc_year × dev_year)，
对每个 dev_year j：
  LDF[j] = avg(  C[i, j+1] / C[i, j]  for all i where C[i, j+1] is not empty )
```

当前引擎能不能做？技术上能：
1. 三角形数据塞进 multi-key lookup table，复合键 `(acc_year|dev_year)` → cumulative claim
2. 外层 Loop 遍历 dev_year
3. Loop body 必须抽成 sub-formula（不支持嵌套 Loop）
4. sub-formula 内部用 fold 累加分子分母
5. 投影阶段再来一组类似结构

**问题在 ergonomic，不在表达能力**：
- 一个简单的 LDF 计算被拆成 3-4 个 sub-formula
- 调试时跨文件跳来跳去
- "忽略空 cell"语义只能靠 conditional 手写 filter
- 写完以后没人想再 review

**目标**：引入一个原生节点，让"对一张表的某一列做带条件的聚合"变成一行配置，从而把链梯法（和其它类似的"对表做 SQL 风格 SELECT SUM/AVG WHERE ..."的场景）直接接住。

---

## 2. 设计

### 2.1 新增节点类型：`NodeTableAggregate`

```go
type TableAggregateConfig struct {
    TableID     string            `json:"tableId"`     // 引用 lookup_tables.id

    // 选择哪些行（filter 子句）
    Filters     []TableFilter     `json:"filters,omitempty"`
    FilterCombinator string       `json:"filterCombinator,omitempty"` // "and"|"or"，默认 "and"

    // 对选中的行做什么聚合
    Aggregate   string            `json:"aggregate"`   // sum|avg|count|min|max|product
    Expression  string            `json:"expression"`  // 列名 或 简单表达式 "col1/col2"

    // 输出端口
    OutputPort  string            `json:"outputPort"`  // 默认 "out"
}

type TableFilter struct {
    Column string `json:"column"`
    Op     string `json:"op"`     // eq|ne|gt|ge|lt|le|in
    // 支持两种值的来源：常量 或 引用其它节点输出端口
    Value     string `json:"value,omitempty"`     // 常量字符串值
    InputPort string `json:"inputPort,omitempty"` // 引用其它节点的端口名（互斥于 Value）
    Negate    bool   `json:"negate,omitempty"`
}
```

### 2.2 一个具体例子：链梯法 LDF 计算

假设三角形数据存在 lookup table `claims_triangle`，结构：

| acc_year | dev_year | cumulative_claim |
|---|---|---|
| 2023 | 1 | 100 |
| 2023 | 2 | 127 |
| 2023 | 3 | 149 |
| 2023 | 4 | 168 |
| 2024 | 1 | 95 |
| 2024 | 2 | 121 |
| 2024 | 3 | 144 |
| 2025 | 1 | 105 |
| 2025 | 2 | 133 |
| 2026 | 1 | 98 |

**当前要做的**（一堆 sub-formula + manual fold）

**用 TableAggregate 之后**：

```jsonc
// LDF₁ = avg of (cumulative[acc, dev=2] / cumulative[acc, dev=1]) for acc in {2023, 2024, 2025}
{
  "type": "tableAggregate",
  "config": {
    "tableId": "claims_triangle",
    "filters": [
      { "column": "dev_year", "op": "eq", "value": "2" }
    ],
    "aggregate": "avg",
    // expression 是一个简单的"该行的 col / lookup(同 acc_year, dev_year-1) 的 col"
    // 这一步本质需要 self-join，下面 §2.3 讨论怎么做
    "expression": "cumulative_claim"  // 第一版只支持单列
  }
}
```

**第一版的限制**：`expression` 只支持单列名，不支持表达式或 self-join。
要做链梯法的 LDF 比例，**第一版的做法**是预先把"每行的 ratio = col[t+1] / col[t]"算好作为 `claims_triangle` 的一列（`development_ratio`），然后 TableAggregate 直接对 `development_ratio` 做 avg。

> 这在实务里其实是常见做法——三角形数据通常是经过预处理的派生表。
> 第二版（v2）再加 `Expression` 表达式支持。

### 2.3 第二版：Expression DSL（不在 v1 范围内）

未来 `expression` 可以接受简单的列运算：

```
"expression": "claims_paid / earned_premium"
```

或者跨行 self-join：

```jsonc
{
  "expression": "this.cumulative_claim / lookup(table=claims_triangle, acc_year=this.acc_year, dev_year=this.dev_year-1).cumulative_claim"
}
```

但这就是一个 mini DSL 了，实现成本高。**v1 刻意不做**，让用户先用预计算列。

### 2.4 Evaluator 行为

```go
func evalTableAggregate(cfg, inputs, tableResolver) (Decimal, error) {
    // 1. 加载完整表 rows（复用 task #037 的 cache 路径）
    rows := tableResolver.GetRows(cfg.TableID) // 已 cache
    
    // 2. 应用 filters
    selected := []map[string]string{}
    for _, row := range rows {
        if matchFilters(row, cfg.Filters, cfg.FilterCombinator, inputs) {
            selected = append(selected, row)
        }
    }
    
    // 3. 提取 expression 列的值（v1 是单列）
    values := []Decimal{}
    for _, row := range selected {
        s, ok := row[cfg.Expression]
        if !ok {
            continue // 缺列就跳过，符合"忽略空 cell"语义
        }
        d, err := decimal.NewFromString(s)
        if err != nil {
            return Zero, fmt.Errorf("table %s row column %s: %v", cfg.TableID, cfg.Expression, err)
        }
        values = append(values, d)
    }
    
    // 4. 聚合
    return aggregate(cfg.Aggregate, values)
}

func aggregate(mode string, values []Decimal) (Decimal, error) {
    switch mode {
    case "sum":     return sumOf(values), nil
    case "product": return productOf(values), nil
    case "count":   return decimal.NewFromInt(int64(len(values))), nil
    case "avg":
        if len(values) == 0 { return Zero, errors.New("avg over empty set") }
        return sumOf(values).Div(decimal.NewFromInt(int64(len(values)))), nil
    case "min":
        if len(values) == 0 { return Zero, errors.New("min over empty set") }
        m := values[0]
        for _, v := range values[1:] { if v.LessThan(m) { m = v } }
        return m, nil
    case "max":
        if len(values) == 0 { return Zero, errors.New("max over empty set") }
        m := values[0]
        for _, v := range values[1:] { if v.GreaterThan(m) { m = v } }
        return m, nil
    }
    return Zero, fmt.Errorf("unknown aggregate: %s", mode)
}
```

### 2.5 Filter 求值

filter 的右值可以是常量或来自其它节点的输出：

```jsonc
{ "column": "acc_year", "op": "eq", "value": "2024" }                    // 常量比较
{ "column": "dev_year", "op": "le", "inputPort": "current_dev_year" }    // 动态比较
```

`inputPort` 引用其它节点连过来的边的端口名，evaluator 在执行时从 `inputs[port]` 读取 Decimal 值，再转字符串和 row[column] 比较。

数值比较时双方都按 Decimal 解析；字符串比较时按字面值。

### 2.6 与 Task #037 的协作

`StoreTableResolver` 已经缓存了 parsed rows。本 spec 需要新增一个公开 API：

```go
// 现有
ResolveTable(ctx, tableID, keyColumns []string, column string) (map[string]string, error)

// 新增（v1 用）
GetRows(ctx context.Context, tableID string) ([]map[string]string, error)
```

`GetRows` 返回所有原始 rows（cached），TableAggregate 自己做 filter。这避免了"为每个 column / keyColumns 组合各 cache 一份"的浪费。

---

## 3. 涉及文件

- `backend/internal/domain/formula.go` — 新增 `NodeTableAggregate` + `TableAggregateConfig` + `TableFilter`
- `backend/internal/engine/evaluator.go` — 新增 `evalTableAggregate`
- `backend/internal/engine/table_resolver.go` — 新增 `GetRows()` 公开接口（task #037 的 internal `getRows` 提升为公开）
- `backend/internal/engine/engine.go` — TableResolver interface 加 `GetRows`
- `backend/internal/parser/`（可选 v1 之后）— 文本编辑器对应的语法
- `backend/internal/engine/table_aggregate_test.go`（新建）— 单元测试

---

## 4. 测试覆盖

### 必须
- [ ] 简单 sum/avg/count 单 filter
- [ ] 多 filter AND
- [ ] 多 filter OR
- [ ] Filter 引用其它节点输出（dynamic value）
- [ ] 链梯法 LDF 计算（用预计算 development_ratio 列的真实 mortality / claims 表）
- [ ] 空 selected set 的 avg/min/max 报错
- [ ] 空 selected set 的 sum 返回 0、product 返回 1、count 返回 0
- [ ] race-free 并发访问

### 不必须（v2）
- Expression DSL（列运算）
- Self-join 风格的跨行引用
- 文本 parser 语法

---

## 5. 验收标准

- [ ] 链梯法的 LDF 计算可以用 1 个 TableAggregate 节点 + 预计算的 ratio 列表达
- [ ] 公式 8 在 spec 002 的覆盖度从 ⚠️ 升级为 ✅
- [ ] 不破坏现有 NodeTableLookup 的行为
- [ ] 单元测试 + race detector 通过
- [ ] codex review 通过

---

## 6. 风险

| 风险 | 缓解 |
|---|---|
| 全表扫描性能差 | task #037 的 parsed-rows cache 已经把"加载 rows"成本降到 ~0；filter 是单次 O(n)，可接受 |
| Filter 表达式注入 | filter 字段都是结构化的 JSON，没有 string interpolation 风险 |
| 数值/字符串混用比较语义混乱 | 第一版只允许：数值列（Decimal-parseable）用 gt/lt/ge/le/eq/ne；字符串列只能 eq/ne |
| 与 sub-formula cache 的交互 | TableAggregate 的结果作为 evaluator 节点输出参与 ResultCache，与现有缓存机制一致，无新风险 |

---

## 7. 不在本 spec 范围内的事

- **Expression DSL** — 列运算 / self-join，留 v2
- **窗口函数** — running total、lag/lead，留 v2 或 v3
- **GROUP BY** — 第一版只能 select-then-aggregate，不能"按 acc_year 分组算每组 LDF"。链梯法需要外层用 Loop 包一层
- **嵌套 TableAggregate** — Aggregate 节点的 filter value 不能是另一个 Aggregate 的输出（依赖图层面会工作，但语义复杂，先不鼓励）
- **Visual editor UI** — 后端先做，前端 task 之后补
- **文本 parser 语法** — 同上
