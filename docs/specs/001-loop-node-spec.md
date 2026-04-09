# 001 Loop 节点规格说明

## 1. 文档状态

- 编号: `001`
- 标题: `Loop 节点规格说明`
- 状态: `Proposed`
- 适用范围: `backend` / `frontend` / `API graph schema`
- 目标版本: `V1`

## 2. 背景

当前公式引擎以 JSON DAG 作为统一表示，支持以下节点类型：

- `variable`
- `constant`
- `operator`
- `function`
- `subFormula`
- `tableLookup`
- `conditional`
- `aggregate`

现有引擎适合表达静态依赖图，但无法直接表达“按期间迭代计算并汇总结果”的场景。保险领域中，这类需求非常常见，例如：

- 逐保单年度计算现值并求和
- 逐月累计保费、给付、费用
- 对一组期间结果取 `sum / avg / max / last`

需要明确的是，当前系统是 DAG 引擎，不支持图内循环依赖。因此本规格中的 `Loop` 不表示一般意义上的 `for/while` 控制流，也不表示可在图中形成环路的节点，而是：

> 一个在单节点内部执行“有界范围迭代 + 子公式求值 + 结果聚合”的节点。

## 3. 设计目标

### 3.1 目标

- 为保险料率、精算、数理场景提供可复用的期间迭代能力
- 不破坏现有 DAG 拓扑排序与并行执行模型
- 前后端统一建模，支持可视化编辑、保存、校验、执行
- 优先支持可安全落地的 V1 能力，避免一开始引入状态机式复杂度

### 3.2 非目标

- 不支持图级别循环依赖
- 不支持通用 `for / while / break / continue`
- 不支持在 V1 中直接实现“上一期结果作为下一期输入”的状态传递
- 不要求 V1 支持文本模式编辑与双向解析

## 4. 适用业务场景

### 4.1 适合 V1 的场景

- 现值求和：`PV = Σ cashflow(t) * discountFactor(t)`
- 每期费用、每期给付、每期保费的总和
- 对分期结果求平均、最大值、最小值、最后一期值
- 对某一子公式按保单年度或月份重复计算

### 4.2 不适合 V1 的场景

- 责任准备金滚动：`reserve[t] = f(reserve[t-1], t)`
- 账户价值滚动
- 带显式中间状态传递的精算递推
- 任意递归/自引用

这类需求应作为 V2 的“状态型 Loop / Fold Loop”单独设计。

## 5. 概念模型

Loop 节点由三部分组成：

1. `range`：定义迭代区间
2. `body`：每次迭代调用的子公式
3. `aggregation`：将各次迭代结果聚合为单一输出

执行语义：

1. 读取 `start`、`end`、`step`
2. 生成有界迭代序列
3. 每轮向目标子公式注入迭代变量
4. 执行子公式并取得单值结果
5. 用指定聚合方式汇总
6. 输出单个 decimal 结果

## 6. V1 规格总览

### 6.1 新增节点类型

在后端 `domain.NodeType` 与前端 `NodeType` 中新增：

```go
NodeLoop NodeType = "loop"
```

### 6.2 Node Config

后端建议新增配置结构：

```go
type LoopConfig struct {
    Mode          string `json:"mode"`                    // 固定为 "range"
    FormulaID     string `json:"formulaId"`               // 必填，循环体子公式 ID
    Version       *int   `json:"version,omitempty"`       // 可选，nil 表示发布版本
    Iterator      string `json:"iterator"`                // 必填，例如 "t" / "m" / "policyYear"
    Aggregation   string `json:"aggregation"`             // sum/product/count/avg/min/max/last
    InclusiveEnd  *bool  `json:"inclusiveEnd,omitempty"`  // 默认 true
    MaxIterations *int   `json:"maxIterations,omitempty"` // 可选，节点级限制
}
```

前端 `FormulaNode.config` 使用同字段结构，保持与后端一致。

### 6.3 输入输出端口

Loop 节点输入端口：

- `start`
- `end`
- `step`

Loop 节点输出端口：

- `out`

其中：

- `start` 必填
- `end` 必填
- `step` 可选，未连接时默认值为 `1`

### 6.4 聚合函数

V1 支持以下聚合函数：

- `sum`
- `product`
- `count`
- `avg`
- `min`
- `max`
- `last`

## 7. 详细语义

### 7.1 范围语义

V1 只支持 `mode = "range"`。

迭代序列生成规则：

- `step > 0` 时，从 `start` 向上迭代到 `end`
- `step < 0` 时，从 `start` 向下迭代到 `end`
- `step = 0` 为非法
- 默认 `inclusiveEnd = true`

示例：

- `start=1, end=5, step=1` -> `1,2,3,4,5`
- `start=1, end=5, step=2` -> `1,3,5`
- `start=5, end=1, step=-2` -> `5,3,1`
- `inclusiveEnd=false` 时，最后一个满足边界的终点值不包含在内

### 7.2 数值类型约束

虽然引擎底层使用 decimal，但 Loop 的迭代边界必须满足以下约束：

- `start` 必须是整数值
- `end` 必须是整数值
- `step` 必须是整数值

若任一值不是整数，则执行时报错。

原因：

- 保单年度、月份、期数本质上是离散索引
- 避免 decimal 步长带来的边界歧义和无限循环风险
- 降低前后端实现复杂度

### 7.3 子公式执行语义

每次迭代时，Loop 节点按以下规则调用子公式：

1. 复制父计算上下文输入
2. 注入一个额外变量，变量名等于 `iterator`
3. 变量值为当前迭代值
4. 调用 `formulaId` 指定的子公式
5. 取得该子公式的单个输出值

约束：

- V1 要求循环体子公式返回单一业务结果；若未来支持多输出，Loop 仍只消费默认单值输出
- 迭代变量注入到子公式输入集中，其值以字符串 decimal 形式传递，与当前 `Calculate` 输入约定一致

### 7.4 聚合语义

#### `sum`

- 返回所有迭代结果之和
- 空结果集为错误

#### `product`

- 返回所有迭代结果连乘
- 空结果集为错误

#### `count`

- 返回成功执行的迭代次数
- 结果为 decimal 整数值

#### `avg`

- 返回算术平均值
- 使用中间精度做除法
- 空结果集为错误

#### `min`

- 返回最小值
- 空结果集为错误

#### `max`

- 返回最大值
- 空结果集为错误

#### `last`

- 返回最后一次迭代结果
- 空结果集为错误

### 7.5 空循环行为

V1 统一规定：

- 当边界与步长组合导致迭代次数为 `0` 时，返回错误

不采用默认 `0` 或 `null` 的原因：

- 保费与精算计算中静默返回 0 风险较高
- 边界配置错误应尽快暴露
- 有助于测试与审计

### 7.6 最大迭代次数

为控制性能与防止错误配置，Loop 必须受最大迭代次数保护。

约束来源：

- 节点级 `config.maxIterations`
- 系统级默认上限，例如 `1000`

执行时规则：

- 若节点配置了 `maxIterations`，优先使用较小值
- 若实际迭代次数超过上限，返回错误

## 8. 图模型与 API Schema 变更

### 8.1 Graph Schema

`FormulaGraph.nodes[].type` 新增 `loop`。

`FormulaGraph.nodes[].config` 当 `type=loop` 时遵循 `LoopConfig`。

### 8.2 Edge Schema

无需新增边模型字段。沿用现有：

```json
{
  "source": "nodeId",
  "target": "nodeId",
  "sourcePort": "out",
  "targetPort": "start|end|step"
}
```

### 8.3 API DTO

现有 `CreateVersionRequest`、`ParseResponse`、`CalculateResponse` 等 DTO 无需变更结构；只需允许 `graph` 中出现 `loop` 节点。

## 9. 后端实现规格

### 9.1 domain

需要修改：

- `backend/internal/domain/formula.go`

新增内容：

- `NodeLoop`
- `LoopConfig`

### 9.2 evaluator

需要修改：

- `backend/internal/engine/evaluator.go`

新增分支：

```go
case domain.NodeLoop:
    return ev.evalLoop(node, inputs)
```

但 V1 不建议将 Loop 全部放入 `Evaluator` 内部完成。原因如下：

- Loop 依赖子公式执行，不只是当前节点纯函数求值
- 当前 `Evaluator` 更适合“已拿到所有输入值后直接计算”的节点
- `subFormula` 目前已经由 engine 层特殊处理，Loop 的性质与其更接近

因此建议实现方式为：

- `Evaluator` 只保留基础校验/纯聚合逻辑辅助函数
- Loop 的主执行放到 `engine` 层，类似 `subFormula` 特殊处理路径

### 9.3 engine

需要修改：

- `backend/internal/engine/engine.go`
- `backend/internal/engine/parallel.go`
- 可能涉及 `Executor` 对特殊节点的分派逻辑

建议实现方案：

1. 在节点执行阶段识别 `NodeLoop`
2. 解析 `LoopConfig`
3. 从当前节点输入中读取 `start/end/step`
4. 展开迭代序列
5. 对每轮迭代：
   - 克隆 `seedInputs`
   - 注入 `iterator`
   - 调用 `formulaResolver.ResolveFormula`
   - 调用内部 `calculateGraph` 或复用 `executeSubFormula` 路径
6. 收集每轮结果
7. 执行聚合
8. 返回单一 decimal 结果

### 9.4 validation

需要修改：

- `backend/internal/parser/validator.go`
- `backend/internal/engine/engine.go` 中现有图级校验

新增校验规则：

- `config.mode` 必须为 `range`
- `config.formulaId` 非空
- `config.iterator` 非空
- `config.aggregation` 为允许值之一
- `targetPort` 只能是 `start/end/step`
- `start` 必须连接
- `end` 必须连接
- `step` 可不连接
- 不允许同一 `targetPort` 重复连线

运行时校验：

- `step != 0`
- `start/end/step` 为整数
- 实际迭代次数 > 0
- 实际迭代次数 <= 最大迭代数

### 9.5 sub-formula call stack / recursion guard

Loop 本质上会重复调用子公式，必须复用现有子公式防递归机制。

要求：

- `Loop.formulaId` 不允许直接或间接引用当前公式形成无限递归
- 若 `Loop` 子公式中再次引用父公式，应被 call stack guard 拒绝

### 9.6 caching

Loop 节点执行将显著增加子公式调用次数，需要明确缓存策略。

建议：

- 沿用现有 graph cache
- 子公式调用若输入完全相同，可命中现有缓存
- 不额外引入 Loop 专属缓存层，先观察性能

### 9.7 intermediates / audit trail

V1 最低要求：

- `CalculateResponse.intermediates` 仍记录 Loop 节点最终结果

增强建议：

- 未来可增加调试模式，输出每轮迭代结果，如：
  - `loopNodeId#1`
  - `loopNodeId#2`
  - `loopNodeId#3`

V1 不强制要求返回逐轮明细，避免污染现有响应结构。

## 10. 前端实现规格

### 10.1 类型定义

需要修改：

- `frontend/src/types/formula.ts`

新增：

- `NodeType` 中加入 `loop`
- `LoopConfig` 类型定义

建议类型：

```ts
export interface LoopConfig {
  mode: 'range'
  formulaId: string
  formulaName?: string
  version?: number
  iterator: string
  aggregation: 'sum' | 'product' | 'count' | 'avg' | 'min' | 'max' | 'last'
  inclusiveEnd?: boolean
  maxIterations?: number
}
```

### 10.2 编辑器节点展示

需要修改：

- `frontend/src/components/editor/NodePalette.tsx`
- `frontend/src/components/editor/FormulaNode.tsx`
- `frontend/src/components/editor/nodeVariants.tsx`
- `frontend/src/components/editor/nodePresentation.ts`

要求：

- 在 NodePalette 中新增 `loop`
- 为 Loop 指定单独颜色与图标
- 命名输入端口显式展示 `Start / End / Step`
- 避免使用匿名端口

节点文案建议：

- 标题显示：`loop`
- 副标题显示：`sum(policyYear)` 或 `last(month)` 这类紧凑信息

### 10.3 默认配置

`defaultNodeConfig('loop')` 建议为：

```ts
{
  mode: 'range',
  formulaId: '',
  iterator: 't',
  aggregation: 'sum',
  inclusiveEnd: true
}
```

### 10.4 属性面板

需要修改：

- `frontend/src/components/editor/NodePropertiesPanel.tsx`

应支持编辑：

- Body Formula
- Version
- Iterator
- Aggregation
- Inclusive End
- Max Iterations

交互要求：

- 当 `formulaId` 为空时显示错误态
- `iterator` 输入应提示仅使用字母、数字、下划线
- `version` 可为空，表示 published

### 10.5 画布连线校验

需要修改：

- `frontend/src/utils/graphValidation.ts`
- `frontend/src/components/editor/FormulaCanvas.tsx`

校验规则：

- Loop 只能接收 `start/end/step`
- `start` 与 `end` 必连
- `step` 可选
- 不允许重复连接同一端口

### 10.6 图文本转换

V1 建议：

- `reactFlowToText` 不支持 Loop
- 如果图中存在 Loop，文本模式切换应阻止，并给出提示

原因：

- 当前文本语法不支持绑定迭代变量和 body 子公式的表达
- 强行支持会使 parser/serializer 复杂度显著上升

前端提示文案建议：

> 当前公式包含 Loop 节点，暂不支持切换到文本模式，请使用可视化模式编辑。

### 10.7 国际化

需要修改：

- `frontend/src/i18n/index.ts`
- `frontend/src/i18n/locales/en.json`
- `frontend/src/i18n/locales/zh.json`
- `frontend/src/i18n/locales/ja.json`

至少新增：

- `editor.loop`
- `editor.iterator`
- `editor.aggregation`
- `editor.inclusiveEnd`
- `editor.maxIterations`
- `editor.start`
- `editor.end`
- `editor.step`

## 11. 错误处理规范

### 11.1 配置错误

示例：

- `loop node missing formulaId`
- `loop node has invalid aggregation "median"`
- `loop node missing iterator`

### 11.2 连线错误

示例：

- `loop node missing "start" input`
- `loop node missing "end" input`
- `duplicate edge on target port "start"`

### 11.3 运行时错误

示例：

- `loop node step cannot be zero`
- `loop node start/end/step must be integers`
- `loop node produced zero iterations`
- `loop node exceeded maxIterations 1000`
- `loop body formula returned no output`

错误风格应与现有 `node %s: ...` 格式保持一致。

## 12. 测试规格

### 12.1 后端单元测试

至少覆盖：

- `sum` 正向场景
- `avg` 正向场景
- `last` 正向场景
- `step` 缺省为 `1`
- `step < 0`
- `step = 0` 错误
- 非整数边界错误
- 空循环错误
- 超出 `maxIterations` 错误
- 子公式递归保护

建议新增测试文件：

- `backend/internal/engine/loop_test.go`
- `backend/internal/parser/validator_loop_test.go`

### 12.2 前端单元测试

至少覆盖：

- `loop` 节点端口定义
- `defaultNodeConfig('loop')`
- 图校验要求 `start/end`
- 阻止无效端口连接
- 包含 Loop 的图不可切换文本模式

建议新增或补充：

- `frontend/src/utils/graphValidation.test.ts`
- `frontend/src/components/editor/...` 对应测试

### 12.3 集成测试

建议新增一组保险业务样例：

- 给定 `n=3`，按 `policyYear=1..3` 计算贴现保费并求和
- 按 `month=1..12` 取最大费用
- 按 `t=1..5` 取最后一期值

## 13. 示例

### 13.1 保费现值求和

业务含义：

- 对保单年度 `1..n`，每期调用 `pv_premium_term` 子公式
- 返回总现值

Loop 配置：

```json
{
  "type": "loop",
  "config": {
    "mode": "range",
    "formulaId": "pv-premium-term",
    "iterator": "policyYear",
    "aggregation": "sum",
    "inclusiveEnd": true,
    "maxIterations": 200
  }
}
```

输入边：

- `start <- 1`
- `end <- n`
- `step <- 1`

### 13.2 月度费用最大值

业务含义：

- 对 `month=1..12` 逐月计算费用
- 返回最大费用月份对应的费用值

聚合使用：

- `aggregation = "max"`

### 13.3 最后一期值

业务含义：

- 对 `t=1..term` 执行子公式
- 返回最后一期结果

聚合使用：

- `aggregation = "last"`

## 14. 与 Aggregate 的关系

需要明确区分：

### `aggregate`

- 输入是一组已经存在的值
- 节点本身不产生迭代变量
- 更像“收集后再汇总”

### `loop`

- 自己负责产生迭代范围
- 每轮调用子公式生成一个值
- 再进行聚合

因此两者不能互相替代。

## 15. 性能与实现取舍

### 15.1 V1 取舍

V1 明确采用以下保守策略：

- Loop 内部逐次执行，不做并发迭代
- 不开放状态累积器
- 不支持文本模式
- 不扩展 API 响应结构

原因：

- 优先保持行为可解释、可测试、易审计
- 避免与现有 DAG 执行器的并行模型冲突
- 将复杂度集中在最有业务价值的场景

### 15.2 未来优化

在确认业务稳定后，可考虑：

- Loop 内迭代并发执行
- Loop 调试明细输出
- 文本语法扩展
- 状态型 Loop / Fold Loop

## 16. V2 方向

V2 可考虑支持“状态累积型 Loop”，以覆盖责任准备金和账户价值滚动场景。

建议方向：

- 新增 `seed` 输入端口
- 新增 `accumulator` 配置
- 每轮将上轮输出传入下轮
- 最终输出 accumulator

示意：

```text
reserve[t] = reserve[t-1] * (1 + i) + premium - benefit - expense
```

这类能力不纳入 V1。

## 17. 落地清单

### 后端

- 在 `domain` 中新增 `NodeLoop` 与 `LoopConfig`
- 在执行引擎中加入 Loop 特殊执行路径
- 在 validator 中加入 Loop 配置和端口校验
- 在 recursion guard 中覆盖 Loop -> subFormula 调用链
- 补充 engine / validator 单元测试

### 前端

- 在类型定义中新增 `loop`
- 在调色板、节点渲染、属性面板中支持 Loop
- 在图校验中加入 Loop 端口规则
- 在文本模式切换中屏蔽含 Loop 图
- 补充 i18n 与测试

### 文档

- 更新 `docs/design.md` 中的节点类型与端口说明
- 更新 `docs/formula-editor-guide.md` 中的用户操作说明

## 18. 决策总结

本规格确定：

- `Loop` 是一个新节点类型，不是图级循环语义
- V1 采用“范围迭代 + 子公式执行 + 聚合”的无状态模型
- V1 不支持文本模式
- V1 不支持上一轮结果传入下一轮的状态递推

这个设计能覆盖保险领域中大量“按期间重复计算并汇总”的需求，同时保持对现有 DAG 架构、校验模型和前后端实现的最小扰动。
