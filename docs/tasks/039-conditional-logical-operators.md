# Task #039: Conditional 节点的 AND/OR/NOT 支持

## Status: done

## 需求

实现 [`docs/specs/003-conditional-logical-operators.md`](../specs/003-conditional-logical-operators.md) 描述的优先级 1 扩展：让 `NodeConditional` 支持复合条件（多比较 + AND/OR/NOT），消除当前必须靠嵌套 Conditional 才能写复合规则的 UX 缺陷。

## 设计

### 配置变更

`backend/internal/domain/formula.go` 的 `ConditionalConfig` 扩展为：

```go
type ConditionalConfig struct {
    // 旧字段（向后兼容）：当 Conditions 为空时使用
    Comparator string `json:"comparator,omitempty"`

    // 新字段：复合条件
    Conditions []ConditionTerm `json:"conditions,omitempty"`
    Combinator string          `json:"combinator,omitempty"` // "and"|"or"，默认 "and"
}

type ConditionTerm struct {
    Op     string `json:"op"`               // eq|ne|gt|ge|lt|le
    Negate bool   `json:"negate,omitempty"` // 整个 term 取反
}
```

**向后兼容**：当 `Conditions` 为空时走旧路径（`Comparator` + `condition`/`conditionRight` 输入端口）。

### 端口命名约定

复合模式下，第 i 个 condition 的左右值通过端口 `condition_i` 和 `conditionRight_i` 接入：

```
condition_0    → 第 0 个比较的左值
conditionRight_0 → 第 0 个比较的右值
condition_1
conditionRight_1
...
thenValue       → if-true 输出值（不变）
elseValue       → if-false 输出值（不变）
```

旧格式继续用 `condition` 和 `conditionRight`。

### Evaluator 行为

```go
if len(cfg.Conditions) > 0 {
    // 新路径
    var ok bool
    for i, term := range cfg.Conditions {
        leftKey  := fmt.Sprintf("condition_%d", i)
        rightKey := fmt.Sprintf("conditionRight_%d", i)
        left, l := inputs[leftKey]
        right, r := inputs[rightKey]
        if !l || !r { return Zero, error("missing port") }
        cmp := compare(left, term.Op, right)
        if term.Negate { cmp = !cmp }
        if i == 0 {
            ok = cmp
        } else if cfg.Combinator == "or" {
            ok = ok || cmp
        } else {
            ok = ok && cmp  // 默认 and
        }
    }
    if ok { return inputs["thenValue"] }
    return inputs["elseValue"]
}
// 否则旧路径不变
```

### Validator

`engine.go` 的 `Validate()` 同步增加：
- 如果 `Conditions` 非空：每个 term 的 `Op` 必须合法；`Combinator` 必须是 `"and"`/`"or"`/`""`
- 如果 `Conditions` 非空：必须有 `condition_0..N-1` 和 `conditionRight_0..N-1` 入边
- 如果 `Conditions` 为空：旧 `Comparator` 字段必须合法；必须有 `condition`/`conditionRight` 入边
- 不论哪条路径：必须有 `thenValue`/`elseValue` 入边

## 涉及文件

- `backend/internal/domain/formula.go` — 扩展 `ConditionalConfig`，新增 `ConditionTerm`
- `backend/internal/engine/evaluator.go` — 重写 `evalConditional`
- `backend/internal/engine/engine.go` — 更新 conditional 的 config validation 和 input port validation
- `backend/internal/engine/conditional_test.go` （新建）— 单元测试

**不在本 task 范围**：前端 visual editor UI、文本 parser 语法（留作后续 task）

## TODO

- [x] 修改 `domain.ConditionalConfig` + 新增 `ConditionTerm`
- [x] 重写 `evaluator.evalConditional`，支持双路径
- [x] 更新 `engine.Validate()` 中 conditional 的 config 校验和 input port 校验
- [x] 更新 `parser.validateConditional()`（codex round 1 P1）— 复合 conditional 也能通过 import 校验
- [x] 更新 `parser.dagToASTWalk()` Conditional 分支（codex round 2 P2）— 复合 conditional 显式拒绝并指引使用 visual editor，UX 与现有 loopNoTextMode 一致
- [x] 写单元测试覆盖：单 AND / 单 OR / 多条件 AND / 多条件 OR / NOT / AND+NOT / 旧 config 回归 / 端口缺失错误 / parser validator / DAGToAST 拒绝
- [x] `go vet ./... && go test ./... -race` 通过
- [x] codex review 三轮 → P1+P2 全部修复 → LGTM
- [x] 更新 backlog（移到已完成）

## 已知限制（留作后续 task）

- **文本编辑器不支持复合 conditional**：lexer/parser/AST 都没有 boolean combinator 语法，DAGToAST 主动报错让前端检测后切到 visual-only 模式（类似 `editor.loopNoTextMode`）。前端检测：error 字符串包含 `composite conditional` + `visual editor`。
- **混合 AND/OR**：单个 Conditional 节点的 combinator 是 uniform 的，混合需要嵌套两层 Conditional。这是刻意的简化设计。
- **可视化编辑器 UI**：本 task 只做后端，前端添加 condition 列表的 UI 是单独 task。当前可以通过手写 JSON / API 调用使用复合 conditional。

## 完成标准

- [ ] 旧的单 Conditional 公式（task #023 的 IF / 模板里的 if-else）继续工作，无回归
- [ ] 新的复合 Conditional 可以表达：`if loss_ratio > 0.5 AND release < cap then X else 0`
- [ ] race detector + 单元测试全绿
- [ ] codex review LGTM
