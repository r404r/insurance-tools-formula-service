# 003 — Conditional 节点的逻辑运算符（AND / OR / NOT）

**关联文档**：[002-Japan-insurance-coverage-analysis.md](./002-Japan-insurance-coverage-analysis.md) §4 优先级 1
**优先级**：🥇 最高
**预估工作量**：1 天
**状态**：spec — 待实现

---

## 1. 背景与动机

当前 `NodeConditional`（`backend/internal/engine/evaluator.go`）只支持单个比较：

```
if  <left>  <op>  <right>  then  <thenExpr>  else  <elseExpr>
```

`<op>` 限定为 `eq / ne / gt / ge / lt / le` 之一。这在 80% 的场合够用，但下面这类业务规则就要嵌套：

```
异常危险准备金取崩条件（公式 19）：
  if loss_ratio > 0.5 AND cumulative_release < release_cap
  then release_amount
  else 0
```

当前只能写成：

```
if loss_ratio > 0.5
then (if cumulative_release < release_cap then release_amount else 0)
else 0
```

后果：
- 公式图变成"三角形结构"（每多一个 AND 多嵌套一层）
- 文本编辑器里的可读性差
- 任何 N 元 AND/OR 都被迫拆成 N 层嵌套
- 代码 review 和测试都更难

**目标**：让一个 `NodeConditional` 能用 AND/OR/NOT 组合多个比较，无需嵌套。

---

## 2. 设计

### 2.1 选项对比

| 方案 | 描述 | 优点 | 缺点 |
|---|---|---|---|
| **A. Boolean 节点** | 新增 `NodeLogicAnd / Or / Not` + `NodeComparison`，Conditional 的 if 输入接 boolean 节点 | 最灵活，可任意嵌套混合 AND/OR | 需要 4 个新 node type、新 evaluator 路径、新 visual editor UI、新文本语法、Boolean 中间值类型 |
| **B. Conditions 列表 + 单 combinator** | Conditional config 加 `conditions: [{leftId, op, rightId}, ...]` 和 `combinator: "and"\|"or"`，所有 condition 用同一个 combinator 连接 | 改动小、向后兼容、覆盖 80% 场景 | 不能在一个 Conditional 内混合 AND 和 OR；NOT 通过 op 反转实现 |
| **C. JSON 谓词树** | Conditional 的 condition 字段变成嵌套 JSON `{op: "and", children: [...]}` | 表达能力最强 | 谓词不在 graph 里，可视化编辑器看不到，违反 DAG-as-truth 原则 |

### 2.2 选定方案：B + 一个针对 NOT 的小扩展

**核心改动**：扩展 `ConditionalConfig`：

```go
// 旧（当前）
type ConditionalConfig struct {
    Op        string `json:"op"`        // eq/ne/gt/ge/lt/le
    LeftPort  string `json:"leftPort"`  // condLeft
    RightPort string `json:"rightPort"` // condRight
    ThenPort  string `json:"thenPort"`  // thenValue
    ElsePort  string `json:"elsePort"`  // elseValue
}

// 新（向后兼容）
type ConditionalConfig struct {
    // —— 新字段：复合条件 ——
    Conditions []ConditionTerm `json:"conditions,omitempty"` // 若为空，回退到旧字段
    Combinator string          `json:"combinator,omitempty"` // "and"|"or"，默认 "and"

    // —— 旧字段（兼容现有公式）——
    Op        string `json:"op,omitempty"`
    LeftPort  string `json:"leftPort,omitempty"`
    RightPort string `json:"rightPort,omitempty"`

    // —— 通用 ——
    ThenPort string `json:"thenPort"`
    ElsePort string `json:"elsePort"`
}

type ConditionTerm struct {
    Op        string `json:"op"`         // eq/ne/gt/ge/lt/le
    LeftPort  string `json:"leftPort"`   // 输入端口名（连边时填的标签）
    RightPort string `json:"rightPort"`
    Negate    bool   `json:"negate,omitempty"` // 单项 NOT —— 应用 op 后再取反
}
```

**关键决定**：
- **向后兼容**：当 `Conditions` 数组为空时，按旧字段 `Op/LeftPort/RightPort` 解释（一条单比较）；当数组非空时，忽略旧字段
- **Combinator 唯一**：一个 Conditional 内所有 condition 用同一个 combinator（and 或 or）。如果业务需要混合（A AND (B OR C)），用嵌套 Conditional——这是 20% 的场景，刻意不支持以保持 ergonomic
- **NOT 内联在 condition term**：`Negate: true` 表示对整个比较取反。这让 `(NOT A) AND B` 这种常见模式不需要新建节点
- **端口名复用现有边模型**：每个 ConditionTerm 引用两个 input port name（已经是当前的边连接方式），不需要改图模型

### 2.3 Evaluator 行为

```go
// 伪代码
func evalConditional(cfg, inputs) Decimal {
    var ok bool
    if len(cfg.Conditions) > 0 {
        // 新路径：复合条件
        ok = (cfg.Combinator == "and") // and 起始 true，or 起始 false
        for i, term := range cfg.Conditions {
            left  := inputs[term.LeftPort]
            right := inputs[term.RightPort]
            result := compare(left, term.Op, right)
            if term.Negate {
                result = !result
            }
            if i == 0 {
                ok = result
            } else if cfg.Combinator == "or" {
                ok = ok || result
            } else { // "and" 或空（默认 and）
                ok = ok && result
            }
        }
        // 短路优化（可选）：发现 ok 已经决定时提前返回
    } else {
        // 旧路径：单比较
        ok = compare(inputs[cfg.LeftPort], cfg.Op, inputs[cfg.RightPort])
    }
    if ok {
        return inputs[cfg.ThenPort]
    }
    return inputs[cfg.ElsePort]
}
```

**短路求值**：因为所有 input 在 evaluator 收到时已经 evaluated 完毕（DAG 是 levels-based），运行时短路求值不能避免上游计算，但可以避免下游 conditional 内部的 compare 调用。属于微优化，不强制。

### 2.4 端口模型

每个 ConditionTerm 引用两个端口名。端口名是现有边模型里的 `targetHandle`（visual editor 拖线时填的字段）。例：

```
Condition #0:  leftPort="cond0_left",  rightPort="cond0_right",  op="gt"
Condition #1:  leftPort="cond1_left",  rightPort="cond1_right",  op="lt"
```

这意味着 Conditional 节点上有 2N+2 个 input ports（N 个条件 × 2 端 + then + else）。端口名由 visual editor 在创建/编辑 Condition 时自动分配（cond0_left、cond0_right、cond1_left、...）。

### 2.5 Validator

`Validate(graph)` 需要新增检查：
- 如果 `Conditions` 非空，则 `Combinator` 必须是 `"and"` / `"or"` / `""`（空 == and）
- 每个 ConditionTerm 的 `Op` 必须是合法比较运算符
- 每个 ConditionTerm 的 `LeftPort` / `RightPort` 必须有对应的入边
- `ThenPort` / `ElsePort` 必须有对应的入边
- 如果 `Conditions` 非空，旧字段 `Op/LeftPort/RightPort` 应该被忽略并 warn（不报错，向后兼容）

---

## 3. 涉及文件

### 后端
- `backend/internal/domain/formula.go` — `ConditionalConfig` 加新字段 + `ConditionTerm` 类型
- `backend/internal/engine/evaluator.go` — `evalConditional` 改造
- `backend/internal/engine/engine.go`（如有 validator）— 新增 validator 规则
- `backend/internal/engine/conditional_test.go`（新建）— 单元测试覆盖：
  - 单 AND（兼容）
  - 单 OR
  - 多条件 AND
  - 多条件 OR
  - NOT 单项
  - AND + NOT 组合
  - 旧 config 仍然工作（regression）

### 前端（本 task **不涉及**，留作后续 task）
- 视觉编辑器目前应该已经有 ConditionalNode 组件；改 UI 让用户能添加多个 condition 是单独工作量
- 文本编辑器的语法扩展（`if A and B then ...`）也是单独工作量
- **本 task 只做后端**：API 支持复合 conditions，前端可以通过手写 JSON 公式来用上

### 文本解析器（可选）
- `backend/internal/parser/` 的 Pratt parser 当前支持 `if/then/else`，但 condition 部分只接受单比较
- 扩展为接受 `and` / `or` / `not` 关键字 → 对应 ConditionTerm 列表
- 如果时间紧，本 task **可以不做** parser 扩展，只让 API + visual editor (JSON 手写) 支持

---

## 4. 验收标准

### 必须
- [ ] 旧的 Conditional 公式（单比较）继续工作，无回归
- [ ] 新的 Conditional 公式可以用 `Conditions: [...] + Combinator: "and"/"or"` 表达 N 元逻辑
- [ ] 单项 `Negate: true` 工作正确
- [ ] 复合条件公式可以正确计算公式 19 的取崩条件
- [ ] 单元测试覆盖以下场景，全部 race-free 通过：
  - 单 AND、单 OR、单 NOT、AND + NOT、OR + NOT、N=3 AND、N=3 OR
  - 旧 config 回归

### 不必须（留作后续）
- 文本 parser 扩展（`a and b` 语法）
- 视觉编辑器 UI 增加 "Add Condition" 按钮
- 短路求值优化

---

## 5. 不在本 spec 范围内的事

- **Boolean 中间值类型**：本 spec 没有引入 boolean 类型，所有比较结果还是 Decimal 0/1 在内部用，不暴露给用户
- **混合 AND/OR**：必须靠嵌套 Conditional，刻意不在一个节点内支持
- **比较以外的谓词**（如 `is_null`、`is_table_member`）：未来可能加，但本 spec 不做
- **三元布尔逻辑**（true/false/unknown）：不引入

---

## 6. 风险

| 风险 | 缓解 |
|---|---|
| 旧 Conditional 公式被新代码误判为新格式 | 严格判断 `len(Conditions) > 0`，只有非空才走新路径 |
| 端口名冲突 | visual editor 在创建 ConditionTerm 时自动用 `cond{i}_{left/right}` 命名空间 |
| Validator 漏检导致运行时 panic | 单元测试覆盖每种字段缺失/非法值组合 |
| 前后端格式不一致 | 后端先做、留接口契约文档；前端 task 之后再补 UI |
