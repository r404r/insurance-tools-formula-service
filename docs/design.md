# Insurance Formula Calculation Engine - Design Document

## 1. Architecture Overview

```
┌─────────────────────────────────────────────────────┐
│                   Frontend (React)                   │
│  ┌──────────┐  ┌──────────┐  ┌────────┐  ┌───────┐ │
│  │  Formula  │  │ Version  │  │  Auth  │  │ i18n  │ │
│  │  Editor   │  │ Manager  │  │ Pages  │  │zh/ja/en│ │
│  │(react-flow)│ │(timeline)│  │(login) │  │       │ │
│  └──────────┘  └──────────┘  └────────┘  └───────┘ │
│         │              │            │                │
│    Zustand + TanStack Query (State Management)       │
│         │              │            │                │
│              REST API Client (fetch/axios)            │
└──────────────────────┬──────────────────────────────┘
                       │ HTTP/JSON
┌──────────────────────▼──────────────────────────────┐
│                  Backend (Go + chi)                   │
│  ┌─────────────────────────────────────────────────┐ │
│  │              API Layer (chi router)              │ │
│  │  auth_handler │ formula_handler │ calc_handler   │ │
│  │  version_handler │ batch_handler │ table_handler │ │
│  └─────────┬────────────┬─────────────┬────────────┘ │
│            │            │             │              │
│  ┌─────────▼──┐  ┌──────▼──────┐  ┌──▼───────────┐ │
│  │   Auth     │  │  Calculation │  │   Store      │ │
│  │  (RBAC)    │  │   Engine     │  │ (Repository) │ │
│  │  JWT+Roles │  │  DAG+Parallel│  │ SQLite/PG/My │ │
│  └────────────┘  └──────┬──────┘  └──────────────┘ │
│                         │                            │
│                  ┌──────▼──────┐                     │
│                  │   Parser    │                     │
│                  │ AST↔DAG↔Text│                     │
│                  └─────────────┘                     │
└──────────────────────────────────────────────────────┘
```

## 2. Core Data Model: Formula as DAG

公式的核心表示是 **JSON DAG（有向无环图）**，这一结构同时服务于三个目的：

1. **存储格式** — 以 JSON 存储在数据库的 `graph_json` 字段中
2. **可视化展示** — 直接映射到 react-flow 的节点和边
3. **执行计划** — DAG 边关系直接定义了计算依赖和并行机会

### 2.1 DAG Schema

```json
{
  "nodes": [
    {
      "id": "string",
      "type": "variable|constant|operator|function|subFormula|tableLookup|conditional|aggregate",
      "config": { /* type-specific configuration */ }
    }
  ],
  "edges": [
    {
      "source": "nodeId",
      "target": "nodeId",
      "sourcePort": "out",
      "targetPort": "left|right|in|key|condition|thenValue|elseValue|items"
    }
  ],
  "outputs": ["nodeId"],
  "layout": {
    "positions": { "nodeId": {"x": 0, "y": 0} }
  }
}
```

### 2.2 Node Types and Ports

| Node Type | Input Ports | Output Port | Config |
|-----------|------------|-------------|--------|
| variable | (none) | out | name, dataType |
| constant | (none) | out | value |
| operator | left, right | out | op (add/subtract/multiply/divide/power/modulo) |
| function | in (+ args) | out | fn, args |
| subFormula | (mapped) | out | formulaId, version |
| tableLookup | key | out | tableId, lookupKey, column |
| conditional | condition, thenValue, elseValue | out | comparator |
| aggregate | items | out | fn (sum/product/count/avg), range |

## 2.5 Formula Creation & Editing Workflow

```
用户(Admin/Editor)
    │
    ├─ 1. 公式列表页 → 点击「新建公式」
    │     └─ 弹窗输入: 名称、领域(life/property/auto)、描述
    │     └─ POST /api/v1/formulas → 创建公式(无版本)
    │     └─ 跳转到编辑器 /formulas/:id
    │
    ├─ 2. 可视化编辑器
    │     ├─ 左侧: NodePalette(8种节点类型拖拽)
    │     ├─ 中央: FormulaCanvas(react-flow画布)
    │     │     └─ 拖入节点 → 连线 → 自动布局
    │     ├─ 右侧: NodePropertiesPanel(选中节点配置)
    │     └─ 底部: 测试面板(JSON输入 → 计算结果)
    │
    ├─ 3. 保存 → POST /api/v1/formulas/:id/versions
    │     └─ 前端将 react-flow 图转为 API DAG JSON
    │     └─ 位置信息存储在 layout.positions 中
    │     └─ 自动计算输出节点(无出边的节点)
    │     └─ 创建新版本(Draft状态)，版本号自动递增
    │
    └─ 4. 发布流程
          └─ 版本页面 → 发布(Draft→Published) / 归档(→Archived)
          └─ 同一公式只有一个 Published 版本
```

## 3. Dual-Mode Editor Design

### 3.1 Visual Mode (react-flow)

- 用户通过侧边栏拖拽节点类型到画布
- 连线自动基于端口类型验证兼容性
- 选中节点在属性面板中编辑配置
- dagre 自动布局避免节点重叠

### 3.2 Text Mode

- Pratt 解析器支持标准数学表达式语法
- 示例：`round(lookup(mortality, age) * sumAssured, 18)`
- 支持 if/then/else 条件表达式

### 3.3 Mode Switching

```
Visual Mode (react-flow nodes/edges)
        ↕ graphSerializer.ts (前端转换)
API Format (JSON DAG)
        ↕ parser/serializer.go (后端转换)
Text Mode (expression string)
        ↕ parser/parser.go (后端解析)
AST (Abstract Syntax Tree)
```

切换流程：
1. Visual → Text: 前端将 react-flow 图序列化为 API DAG JSON，调用后端 DAG→Text 转换 API
2. Text → Visual: 前端发送文本到后端解析 API，后端返回 DAG JSON，前端反序列化为 react-flow

## 4. Parallel Execution Engine

### 4.1 Algorithm

```
Input: FormulaGraph (nodes + edges)
  ↓
Step 1: Build adjacency lists from edges → O(E)
  ↓
Step 2: Kahn's topological sort → identify independent levels → O(V+E)
  ↓
Step 3: Level-based parallel dispatch
  Level 0: [variable nodes]     → seed from inputs
  Level 1: [independent nodes]  → parallel goroutines
  Level 2: [dependent nodes]    → parallel goroutines (read Level 1 results)
  ...
  Level N: [output nodes]       → collect results
```

### 4.2 Concurrency Model

- `errgroup.WithContext` 管理每层级的 goroutine 组
- `sync.Map` 存储已完成节点的结果（每层只读前一层结果，无竞争）
- Worker 池大小 = `runtime.NumCPU()`，可配置
- 节点数 < 8 时退化为串行执行

### 4.3 Result Caching

- LRU 缓存 keyed by `(formulaID, version, inputHash)`
- 批量计算场景下，相同子公式的结果可复用
- 缓存大小可配置（默认1000条）

## 5. High Precision Design

- **库**: `shopspring/decimal` — 金融级定点十进制运算
- **中间精度**: 28位小数（可配置），确保中间计算不丢精度
- **输出精度**: 18位小数（可配置），最终结果按需截断
- **API传输**: 数值以字符串格式传输，避免 JSON float64 精度丢失
- **前端显示**: 使用 `bignumber.js` 格式化显示，不在前端做计算

## 6. Version Management Design

### 6.1 State Machine

```
           create
    ──────────→ Draft
                  │
          publish │
                  ↓
              Published ←── (only one per formula)
                  │
          archive │       create new draft (copy)
                  ↓              ↓
              Archived         Draft (new version)
```

### 6.2 Storage Strategy

- **全量快照**: 每个版本存储完整的 `FormulaGraph` JSON
- **Diff 按需计算**: 比较两个版本的节点/边集合，返回 added/removed/modified
- **回滚**: 复制目标版本的 `graph_json` 创建新 Draft，`parent_ver` 指向目标版本

## 7. RBAC Design

### 7.1 Role Hierarchy

| Permission | Admin | Editor | Reviewer | Viewer |
|-----------|-------|--------|----------|--------|
| View Formulas | Y | Y | Y | Y |
| Calculate | Y | Y | Y | Y |
| Create/Edit Formula | Y | Y | - | - |
| Delete Formula | Y | Y | - | - |
| Publish/Archive Version | Y | - | Y | - |
| Manage Tables | Y | Y | - | - |
| Manage Users | Y | - | - | - |

### 7.2 Auth Flow

1. 用户注册/登录 → 获取 JWT token
2. 请求携带 `Authorization: Bearer <token>`
3. Auth middleware 验证 token，注入 Claims 到 context
4. Permission middleware 检查角色权限

## 8. Database Design

### 8.1 Entity Relationship

```
users (1) ──→ (N) formulas
users (1) ──→ (N) formula_versions
formulas (1) ──→ (N) formula_versions
lookup_tables (独立实体，被 tableLookup 节点引用)
```

### 8.2 Database Abstraction

```go
Store interface
  ├── Formulas() FormulaRepository
  ├── Versions() VersionRepository
  ├── Users()    UserRepository
  ├── Tables()   TableRepository
  ├── Migrate()
  └── Close()

Implementations:
  ├── sqlite/store.go    (modernc.org/sqlite, pure Go)
  ├── postgres/store.go  (jackc/pgx/v5)
  └── mysql/store.go     (go-sql-driver/mysql)
```

## 9. API Design

### 9.1 Endpoint Summary

| Method | Path | Auth | Permission | Description |
|--------|------|------|-----------|-------------|
| POST | /api/v1/auth/login | - | - | 用户登录 |
| POST | /api/v1/auth/register | - | - | 用户注册 |
| GET | /api/v1/auth/me | Y | - | 当前用户信息 |
| GET | /api/v1/formulas | Y | View | 公式列表 |
| POST | /api/v1/formulas | Y | Edit | 创建公式 |
| GET | /api/v1/formulas/:id | Y | View | 获取公式 |
| PUT | /api/v1/formulas/:id | Y | Edit | 更新公式 |
| DELETE | /api/v1/formulas/:id | Y | Edit | 删除公式 |
| GET | /api/v1/formulas/:id/versions | Y | View | 版本列表 |
| POST | /api/v1/formulas/:id/versions | Y | Edit | 创建版本 |
| GET | /api/v1/formulas/:id/versions/:ver | Y | View | 获取版本 |
| PATCH | /api/v1/formulas/:id/versions/:ver | Y | Publish | 更新版本状态 |
| GET | /api/v1/formulas/:id/diff | Y | View | 版本Diff |
| POST | /api/v1/calculate | Y | Calculate | 单次计算 |
| POST | /api/v1/calculate/batch | Y | Calculate | 批量计算 |
| POST | /api/v1/calculate/validate | Y | Calculate | 公式验证 |
| GET | /api/v1/tables | Y | View | 查找表列表 |
| POST | /api/v1/tables | Y | Edit | 创建查找表 |
| GET | /api/v1/tables/:id | Y | View | 获取查找表 |
| GET | /api/v1/users | Y | Manage | 用户列表 |
| PATCH | /api/v1/users/:id/role | Y | Manage | 更新用户角色 |

### 9.2 Calculation Request/Response

```json
// POST /api/v1/calculate
// Request
{
  "formulaId": "uuid",
  "version": 3,
  "inputs": { "age": "35", "sumAssured": "1000000.00" },
  "precision": 18
}

// Response
{
  "result": { "n5": "12345.678901234567890000" },
  "intermediates": { "n3": "0.001234", "n4": "1234.000000" },
  "executionTimeMs": 2.4,
  "nodesEvaluated": 12,
  "parallelLevels": 4
}
```

## 10. Insurance Domain Templates

### 10.1 Life Insurance (人寿保险)

- **生命表查询**: 根据年龄查找死亡率 qx
- **净保费计算**: Premium = Sum Assured × qx × discount factor
- **准备金计算**: Zillmer/CRVM 方法，递推公式
- **年金因子**: ax = Σ(vt × tpx), 基于生命表和利率

### 10.2 Property Insurance (财产保险)

- **风险评分**: 多因子加权/乘积模型（建筑类型、地理位置、用途、防护等级）
- **费率计算**: Premium = Base Rate × Risk Score × Sum Insured × Adjustments
- **赔付率**: Loss Ratio = Incurred Losses / Earned Premium

### 10.3 Auto Insurance (车险)

- **车辆分类**: 按品牌/型号/年份/排量分组
- **驾驶员风险**: 年龄、驾龄、出险记录、违章记录多维评级
- **无赔款优惠(NCD)**: 无赔年数 → 折扣等级状态机，出险 → 回退N级
- **保障计算**: 交强险/商业三者/车损/不计免赔各自独立计算
