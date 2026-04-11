# Task #040: 表聚合节点（Table Aggregate Node）

## Status: done

## 需求

实现 [`docs/specs/004-table-aggregate-node.md`](../specs/004-table-aggregate-node.md) 描述的优先级 2 扩展：引入原生 `NodeTableAggregate` 节点，让"对一张表的某一列做带条件的聚合"变成一行配置，解锁链梯法（公式 8）和其它"对历史数据做带条件聚合"的场景。

## 设计

### v1 范围（本 task）

按 spec 004 §2 的 v1 范围：
- 新节点 `NodeTableAggregate`，配置 `TableAggregateConfig`：
  - `TableID`（必填）
  - `Filters []TableFilter`（可选）+ `FilterCombinator`（"and"/"or"，默认 "and"）
  - `Aggregate`（必填，sum/avg/count/min/max/product 之一）
  - `Expression`（必填，**v1 只支持单列名**）
- `TableFilter` 支持常量值和动态值（引用其它节点输出端口）
- 节点 evaluator 直接读取整张表的 cached parsed rows（task #037 的复用）
- 全部 v2/v3 特性（Expression DSL、self-join、嵌套 TableAggregate）**不在本 task 范围**

### Evaluator 集成

`Evaluator` struct 增加 `TableResolver` 字段，由 `NewExecutor` 在构造时注入。这样
evaluator 可以在运行时调用 `tableResolver.GetRows(ctx, tableID)` 拿到全量行数据。

`TableResolver` 接口扩展：

```go
type TableResolver interface {
    ResolveTable(ctx context.Context, tableID string, keyColumns []string, column string) (map[string]string, error)
    GetRows(ctx context.Context, tableID string) ([]map[string]string, error)  // NEW
}
```

`StoreTableResolver` 已经在 task #037 内部 cache 了 parsed rows——把内部 `getRows` 方法包装成公开的 `GetRows`（公开版本签名加 `ctx context.Context`，内部仍走相同的 cache 路径）。

### Filter 求值

每个 filter 在 evaluator 运行时执行：

```go
for _, row := range rows {
    if matchFilters(row, cfg.Filters, cfg.FilterCombinator, inputs) {
        // 选中
    }
}
```

`matchFilters` 对每个 filter：
- 如果 `InputPort != ""`，从 `inputs[InputPort]` 读 Decimal 值，转字符串比较
- 否则 `Value` 是常量字符串
- 用 `Op`（eq/ne/gt/ge/lt/le）做比较：
  - 数值列（`row[Column]` 可以解析为 Decimal）→ 走 Decimal 比较
  - 否则字符串列只允许 eq/ne，gt/ge/lt/le 报错
- `Negate` 翻转结果

### Aggregate 计算

对选中行的 `Expression` 列值（v1 只支持单列名）：
- 解析为 Decimal，跳过解析失败/缺失的行
- 应用聚合：sum/avg/count/min/max/product

空 selected set 的语义：
- `count` → 0
- `sum` → 0
- `product` → 1
- `avg` / `min` / `max` → 报错

### 端口模型

`NodeTableAggregate` 的输入端口是动态的：
- 每个 filter 如果用 `InputPort`，那个端口就是入边的 target port name
- 没有"必需端口"列表（如果 filter 全是常量，节点可能没有任何入边）

输出：单值（聚合结果），用默认 "out" 端口连接到下游。

### 涉及文件

- `backend/internal/domain/formula.go` — 新增 `NodeTableAggregate` NodeType + `TableAggregateConfig` + `TableFilter`
- `backend/internal/engine/engine.go`:
  - `TableResolver` 接口增加 `GetRows`
  - `validate()` 添加 `NodeTableAggregate` 的 config 校验
  - `validateGraph()` 不需要硬性 port 校验（端口动态）
- `backend/internal/engine/table_resolver.go` — `StoreTableResolver` 公开 `GetRows()`
- `backend/internal/engine/evaluator.go`:
  - `Evaluator` 加 `TableResolver` 字段
  - 新增 `evalTableAggregate(ctx, node, inputs)` 方法
  - `EvaluateNode` 加 `case NodeTableAggregate`
- `backend/internal/engine/parallel.go` — `NewExecutor` 接受 `TableResolver` 参数；`NewEvaluator` 同步
- `backend/internal/engine/engine.go` `NewEngine` — 把 `cfg.TableResolver` 传给 executor
- `backend/internal/parser/validator.go` — 添加对应 case 校验 config
- `backend/internal/engine/table_aggregate_test.go`（新建）— 单元测试

**不在本 task 范围**：前端 UI、文本 parser 语法、Expression DSL、self-join、嵌套 TableAggregate

## TODO

- [x] 扩展 `domain.NodeType` 加 `NodeTableAggregate`
- [x] 新增 `domain.TableAggregateConfig` + `TableFilter`
- [x] 扩展 `engine.TableResolver` 接口加 `GetRows`
- [x] `StoreTableResolver` 公开 `GetRows`
- [x] 扩展 `Evaluator` 持有 `TableResolver`
- [x] 实现 `evalTableAggregate` 包括 filter + aggregate
- [x] 注册到 `EvaluateNode` switch
- [x] 更新 `NewExecutor` / `NewEvaluator` 签名 + `NewEngine` 传参
- [x] `engine.go` Validate config 校验
- [x] `parser/validator.go` 校验 + 测试
- [x] 单元测试覆盖：sum/avg/count/min/max/product、单 filter、多 filter AND、多 filter OR、动态 input filter、Negate、空集语义、缺失列、链梯法 LDF 真实例子
- [x] `go vet ./... && go test ./... -race` 通过
- [x] codex review 两轮 → P2×2 修复（avg DivRound 精度 + count 语义）→ LGTM
- [x] 移到 backlog 已完成

## Codex review fixes

**Round 1 — 2 个 P2**：

1. **Average 忽略 intermediate precision**：原实现用 `acc.Div(n)`，shopspring 默认走全局 `DivisionPrecision = 16` 位，无视引擎的 `Precision.IntermediatePrecision` 配置。修复：把 `aggregateDecimalValues` 改为 `*Evaluator` 方法，内部用 `acc.DivRound(n, ev.Precision.IntermediatePrecision)`。回归测试 `TestTableAggregate_AvgUsesIntermediatePrecision` 用 30 位自定义精度验证。

2. **count 静默丢弃非数值行**：原实现 count 用 `len(values)`，而 values 是 `decimal.NewFromString` 解析成功后才追加的，导致 count 在文本列上永远返回 0。修复：用 `presentCount` 跟踪通过 filter 且 column 存在的行数（不论是否数值可解析），count 改用 `presentCount`。语义对齐 SQL `COUNT(column)`：missing column = 不计、present + 非数值 = 计、present + 数值 = 计。回归测试 `TestTableAggregate_CountOnTextColumn` 验证。

**Round 2**：LGTM。

## 完成标准

- [ ] 链梯法的 LDF 计算可以用 1 个 TableAggregate 节点 + 预计算 `development_ratio` 列表达
- [ ] 公式 8 的支持等级从 ⚠️ 升级为 ✅
- [ ] 不破坏现有 NodeTableLookup 行为
- [ ] race detector + 单元测试全绿
- [ ] codex review LGTM
