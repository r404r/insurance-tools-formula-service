# Task #023: 生命保険数理 Loop 公式支持

## Status: in-progress

## 需求

支持日本生命保险数理中三个核心公式的编辑和计算：
1. 纯保险料（一时払）：∑ v^t · (t-1)p_x · q_{x+t-1}
2. 年金现値（年払分母）：∑ v^t · tp_x
3. 责任准备金递推：V[t+1] = (V[t]+P)(1+i) - S·q_{x+t}

## 涉及改动

### 改动 1：空迭代默认值（小改动）

**现状**：0 次迭代 → 报错
**目标**：sum→0, product→1, count→0; avg/min/max/last 仍报错（语义上不合理）

**修改范围**：
- `backend/internal/engine/engine.go` — `executeLoop` 中移除 0 迭代报错，改为 `aggregateLoopResults` 处理空切片
- `backend/internal/engine/engine.go` — `aggregateLoopResults` 对空切片返回单位元
- `backend/internal/engine/loop_test.go` — 新增空迭代测试

**风险**：低。仅扩展合法输入范围，不改变已有行为。

### 改动 2：fold 聚合模式（中等改动）

**设计**：新增 `aggregation: "fold"` 模式，配合两个新字段：
```
LoopConfig {
  ...
  aggregation: "fold"
  accumulatorVar: "V"     // 注入到 body 的累积变量名
  initValue: "0"          // 首次迭代的累积初始值
}
```

**执行语义**：
```
acc = initValue
for t = start to end:
    childInputs = clone(seedInputs)
    childInputs[iterator] = t
    childInputs[accumulatorVar] = acc
    acc = body(childInputs)
return acc
```

**修改范围**：
- `backend/internal/domain/formula.go` — LoopConfig 新增 AccumulatorVar / InitValue 字段
- `backend/internal/engine/engine.go` — executeLoop 中分支处理 fold 模式
- `backend/internal/engine/engine.go` — validateNodeConfig 中校验 fold 模式的必填字段
- `backend/internal/engine/loop_test.go` — fold 测试（递推、累积等）
- `frontend/src/types/formula.ts` — LoopConfig 新增字段
- `frontend/src/components/editor/NodePropertiesPanel.tsx` — fold 模式 UI
- `frontend/src/utils/graphValidation.ts` — fold 模式校验
- `frontend/src/utils/graphText.ts` — fold 文本序列化
- `backend/internal/parser/serializer.go` — fold 文本序列化 DAG↔AST
- `frontend/src/utils/formulaLatex.ts` — fold LaTeX 渲染

**风险**：中等。新增执行路径，但与已有 map-reduce 路径隔离（通过 aggregation 值分支），不影响已有功能。

### 改动 3：生命表 dummy 数据

- 创建日本标准生命表（简化版）作为 lookup table 种子数据
- 包含年龄 0-100 的死亡率 q_x

### 改动 4：三个内置公式

1. 纯保险料（一时払）
2. 年金现値（年払分母）
3. 责任准备金递推

### 改动 5：测试数据（30组×3公式）

- 使用 batch test API 验证
- 截图保存
- 测试报告

## TODO

- [ ] 方案确认
- [ ] 改动 1：空迭代默认值
- [ ] 改动 2：fold 聚合模式
- [ ] 改动 3：生命表 dummy 数据
- [ ] 改动 4：三个内置公式
- [ ] codex review
- [ ] 改动 5：截图验证
- [ ] 改动 6：批量测试（30组×3）
- [ ] 测试报告
- [ ] CLAUDE.md 更新测试规则
