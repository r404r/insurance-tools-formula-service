# 003 - Lookup Table 多 Key 支持

**Status**: in-progress
**Created**: 2026-04-06

## 需求

Lookup Table 目前只支持单个 key 列（固定列名 `"key"`）匹配一行。需要扩展为支持多个 key 列的组合查询（复合主键），例如：

```json
[
  {"age": "25", "gender": "M", "qx": "0.00123"},
  {"age": "25", "gender": "F", "qx": "0.00098"},
  {"age": "26", "gender": "M", "qx": "0.00134"}
]
```

节点需要配置 `keyColumns: ["age", "gender"]`，对应每个 key 列有一个独立的输入端口。

## 设计

### 核心规则
- `keyColumns: ["key"]`（单 key）→ 唯一输入端口 id 为 `"key"`（向下兼容现有公式）
- `keyColumns: ["age", "gender"]` → 两个端口 id 分别为 `"age"`、`"gender"`
- 复合键存储格式：`value1|value2`（管道符分隔，按 keyColumns 顺序）
- `KeyColumns` 为空时引擎默认回退为 `["key"]`

### 后端变更
- `domain/formula.go`：`TableLookupConfig` 新增 `KeyColumns []string`，移除 `LookupKey`（未使用）
- `engine/table_resolver.go`：`ResolveTable` 接收 `keyColumns []string`，构造复合键
- `engine/engine.go`：
  - `preloadTableData`：传 `cfg.KeyColumns`（空时回退 `["key"]`）
  - `validateRequiredPorts`：校验每个 key 列端口均已连接
- `engine/evaluator.go`：`evalTableLookup` 读取每个 key 列输入，拼复合键后查表

### 前端变更
- `types/formula.ts`：`TableLookupConfig.keyColumns: string[]`（替换 `lookupKey`）
- `nodePresentation.ts`：
  - `getInputPorts` tableLookup → 动态生成端口（端口 id = 列名）
  - `defaultNodeConfig` tableLookup → `{ tableId: '', keyColumns: ['key'], column: '' }`
- `NodePropertiesPanel.tsx`：key 列可增删的列表 UI

## TODO

- [x] 创建任务文件
- [x] 后端：domain/formula.go 更新 TableLookupConfig
- [x] 后端：engine/table_resolver.go 支持复合键
- [x] 后端：engine/engine.go preloadTableData + validateRequiredPorts
- [x] 后端：engine/evaluator.go 多端口读取
- [x] 前端：types/formula.ts 更新 TableLookupConfig
- [x] 前端：nodePresentation.ts 动态端口
- [x] 前端：NodePropertiesPanel.tsx key 列 UI
- [x] codex review + fix
- [ ] 提交
