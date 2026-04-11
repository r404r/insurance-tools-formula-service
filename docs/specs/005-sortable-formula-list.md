# 005 — 公式列表加「作成者 / 更新者」+ 表头排序

**优先级**：常规
**预估工作量**：~2 天
**状态**：spec — 待实现
**关联 task**：[042-formula-list-sortable.md](../tasks/042-formula-list-sortable.md)

---

## 1. 需求

公式列表页面（`/formulas`）当前只显示「名称 / 分类 / 创建时间 / 操作」4 列，且无法排序。为了让管理员能更快找到「最近改的公式 / 自己改的公式 / 别人改的公式」，需要：

1. 列表新增 2 列：**作成者**（CreatedBy 用户名）+ **更新者**（UpdatedBy 用户名）
2. 表头可点击触发排序（除「操作」列）
3. 排序状态保留在 URL 参数里，刷新页面不丢

## 2. 用户决策（已确认）

| 决策点 | 决定 | 理由 |
|---|---|---|
| **更新者语义** | 「最近一次保存版本的用户」（不是 metadata 编辑者） | 反映实质逻辑变更 |
| **用户名解析** | 后端 LEFT JOIN `users` 表 | 单请求渲染，避免前端二次请求 |
| **NULL 显示** | "—" | 老数据 / 用户被删的兜底 |
| **Sort 状态** | URL params（`?sortBy=name&sortOrder=asc`） | 与现有 search/page 一致 |
| **默认排序** | `updatedAt desc` | 保持当前行为 |

## 3. 后端设计

### 3.1 Domain 模型

```go
type Formula struct {
    ID            string          `json:"id"`
    Name          string          `json:"name"`
    Domain        InsuranceDomain `json:"domain"`
    Description   string          `json:"description"`
    CreatedBy     string          `json:"createdBy"`               // UUID（已有）
    UpdatedBy     string          `json:"updatedBy,omitempty"`     // UUID（新增，可为空）
    CreatedByName string          `json:"createdByName,omitempty"` // 仅 List 填充
    UpdatedByName string          `json:"updatedByName,omitempty"` // 仅 List 填充
    CreatedAt     time.Time       `json:"createdAt"`
    UpdatedAt     time.Time       `json:"updatedAt"`
}
```

`*Name` 字段是 transient 的——只有 `List` 端点才填充，`GetByID` 不填充（避免每次单查都 join）。前端列表渲染依赖这两个字段。

### 3.2 FormulaFilter 加排序

```go
type FormulaFilter struct {
    Domain    *InsuranceDomain
    Search    *string
    Limit     int
    Offset    int
    SortBy    string // "name" | "createdAt" | "updatedAt" | "createdBy" | "updatedBy"; 默认 "updatedAt"
    SortOrder string // "asc" | "desc"; 默认 "desc"
}
```

### 3.3 API endpoint

`GET /api/v1/formulas?sortBy=updatedAt&sortOrder=desc&...`

Handler：
- 验证 `sortBy` 在白名单内（`name|createdAt|updatedAt|createdBy|updatedBy`）；非法值返回 400
- 验证 `sortOrder ∈ {asc, desc}`；非法值返回 400
- 缺省值：`sortBy=updatedAt`，`sortOrder=desc`
- 把验证后的值传给 store

### 3.4 SQLite store SQL

```sql
SELECT f.id, f.name, f.domain, f.description,
       f.created_by, f.updated_by, f.created_at, f.updated_at,
       u1.username AS created_by_name,
       u2.username AS updated_by_name
FROM formulas f
LEFT JOIN users u1 ON u1.id = f.created_by
LEFT JOIN users u2 ON u2.id = f.updated_by
WHERE <filters>
ORDER BY <sortBy> <sortOrder>
LIMIT ? OFFSET ?
```

`<sortBy>` 通过白名单 map **翻译**成 `f.name | f.created_at | f.updated_at | u1.username | u2.username`，**绝不直接拼接**用户输入。NULL 排序：数值/时间用 `ORDER BY col IS NULL, col ...` 模拟 NULLS LAST。

Postgres / MySQL 同步实现。

### 3.5 数据库 schema 迁移

`formulas` 表加 `updated_by TEXT NULL`：

```sql
-- 新装：CREATE TABLE 加上字段
CREATE TABLE IF NOT EXISTS formulas (
    ...
    created_by TEXT NOT NULL REFERENCES users(id),
    updated_by TEXT REFERENCES users(id),  -- 新增
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
)

-- 老装：ALTER TABLE 加字段（容忍 "duplicate column" 错误）
ALTER TABLE formulas ADD COLUMN updated_by TEXT
```

`Migrate()` 函数追加 ALTER TABLE 语句，单独 try 一次，发生 "duplicate column"/"already exists" 类错误时忽略。三个 store backends 都要做。

### 3.6 写入 `updated_by` 的时机

**核心决策**：每次 `POST /formulas/:id/versions` 成功创建新 version 后，由 version handler 回写 `formulas.updated_by = claims.UserID` 和 `formulas.updated_at = now`。

涉及修改：
- `FormulaRepository` 加 `UpdateMeta(ctx, id, updatedBy string, updatedAt time.Time) error` 方法
- `version_handler.go` 的 Create 函数末尾（创建 version 成功后）调用 `h.Formulas.UpdateMeta(...)`

**不在范围**：metadata 更新（PATCH 公式 name/description）也写 `updated_by`。这个未来如果想要可以加，但本 task 只跟踪「最后改 graph 的人」。

## 4. 前端设计

### 4.1 `FormulaList.tsx` 表头改造

```tsx
type SortField = 'name' | 'createdAt' | 'updatedAt' | 'createdBy' | 'updatedBy'

const sortBy = (searchParams.get('sortBy') as SortField) ?? 'updatedAt'
const sortOrder = (searchParams.get('sortOrder') as 'asc' | 'desc') ?? 'desc'

function toggleSort(field: SortField) {
  if (sortBy === field) {
    setSearchParams({ ..., sortBy: field, sortOrder: sortOrder === 'asc' ? 'desc' : 'asc' })
  } else {
    setSearchParams({ ..., sortBy: field, sortOrder: 'desc' })  // 切换字段时默认 desc
  }
}

// 表头 cell 组件
<SortableHeader field="name" current={sortBy} order={sortOrder} onClick={toggleSort}>
  {t('formula.name')}
</SortableHeader>
```

`SortableHeader` 显示 `↑ / ↓ / ⇅` 三种 icon 状态。

### 4.2 列布局

| 列 | i18n key | 排序 | 备注 |
|---|---|---|---|
| Checkbox | — | ✗ | 选择 |
| Name | `formula.name` | ✓ | |
| Domain | `formula.domain` | ✗ | 业务上排序意义不大；保持 filter |
| 作成者 | `formula.createdBy` | ✓ | 新增 |
| 更新者 | `formula.updatedBy` | ✓ | 新增 |
| Created At | `formula.createdAt` | ✓ | |
| Updated At | `formula.updatedAt` | ✓ | 新增（默认排序字段） |
| Actions | `user.actions` | ✗ | 操作列 |

> 注意：增加 2 列后表格更宽。窄屏需要响应式（`Updated At` 可在 `<lg` 隐藏）。本 task 用 Tailwind responsive class 处理，不引入第三方表格库。

### 4.3 React Query 集成

`queryKey` 加 `[sortBy, sortOrder]` 触发 refetch：

```tsx
const { data } = useQuery({
  queryKey: ['formulas', search, domainFilter, page, sortBy, sortOrder],
  queryFn: () => fetchFormulas({ ..., sortBy, sortOrder }),
})
```

### 4.4 i18n keys（en/zh/ja）

```jsonc
"formula": {
  // 已有...
  "createdBy": "Creator" / "创建者" / "作成者",
  "updatedBy": "Updater" / "更新者" / "更新者",
  "updatedAt": "Updated" / "更新时间" / "更新日時",
  "unknownUser": "—"
}
```

## 5. 涉及文件

### 后端
- `backend/internal/domain/formula.go` — `Formula` struct + `FormulaFilter` 扩展
- `backend/internal/store/repository.go` — `FormulaRepository` 接口加 `UpdateMeta`
- `backend/internal/store/sqlite/store.go` — schema + ALTER + List SQL + UpdateMeta
- `backend/internal/store/postgres/store.go` — 同上
- `backend/internal/store/mysql/store.go` — 同上
- `backend/internal/api/formula_handler.go` — List handler 解析 sortBy/sortOrder
- `backend/internal/api/version_handler.go` — Create version 后回写 formulas.updated_by/updated_at
- `backend/internal/store/sqlite/store_test.go` 或新建 — sort 路径测试

### 前端
- `frontend/src/components/shared/FormulaList.tsx` — 表头 + 列 + 排序
- `frontend/src/types/formula.ts` — Formula type 加 updatedBy/createdByName/updatedByName
- `frontend/src/api/client.ts` 或 formula service — 调用接受 sort 参数
- `frontend/src/i18n/locales/{en,zh,ja}.json` — 4 个新 key

## 6. 风险

| 风险 | 缓解 |
|---|---|
| 老数据 `updated_by` 全 NULL | LEFT JOIN 兜底 NULL，前端显示 "—" |
| Sort by 用户名时 NULL 排序顺序混乱 | SQL 层 NULLS LAST 模拟 |
| SQL 注入（用户传 sortBy="x; DROP TABLE"） | 白名单翻译，永不字符串拼接 |
| 列宽溢出 | Tailwind responsive 隐藏次要列 |
| 已有公式 List 的回归 | 现有测试 `TestFormulaList_*` 不应受影响（默认参数保持原行为） |
| Migration 在已部署 SQLite 上失败 | ALTER TABLE 容忍 duplicate column 错误 |
