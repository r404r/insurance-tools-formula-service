# 006 — 编辑「已归档」版本，保存后生成最新版本

**优先级**：常规
**预估工作量**：~1.5 天
**状态**：spec — 待实现
**关联 task**：[043-archived-version-fork-edit.md](../tasks/043-archived-version-fork-edit.md)

---

## 1. 需求

公式版本历史里，已归档（archived）的版本目前是只读的——用户找不到入口去基于一个老归档版本继续编辑。需要：

1. 版本历史页每行加「编辑」按钮，所有状态（draft / published / archived）都可见
2. 点编辑 archived 版本时，弹一次确认对话框告知用户「将基于此版本生成新版本，原归档版本不变」
3. 编辑器接受 `?baseVersion=N` URL 参数，加载该版本的 graph
4. 编辑器顶部显示 fork mode banner，明确告知正在基于哪个版本编辑、保存后会生成 v?
5. 保存时后端创建新 draft 版本，`ParentVersion = baseVersion`，原版本保持不变

## 2. 用户决策（已确认）

| 决策点 | 决定 | 理由 |
|---|---|---|
| **确认对话框** | 弹一次确认（仅 archived 触发） | 防误操作，但 draft/published 不打扰 |
| **Edit 按钮可见性** | 全部状态都显示 | 一致 UX |
| **Fork banner 位置** | 编辑器顶部（持续可见） | 不像弹窗会被关掉而忘记 |
| **Lineage UI** | 仅后端记录 ParentVersion | UI 增强留未来 task |

## 3. 后端设计

### 3.1 CreateVersionRequest 加 baseVersion

```go
type CreateVersionRequest struct {
    Graph       FormulaGraph `json:"graph"`
    ChangeNote  string       `json:"changeNote"`
    BaseVersion *int         `json:"baseVersion,omitempty"` // 新增，可选
}
```

### 3.2 Version handler Create 逻辑

```go
// 当前
nextVersion := maxVersion + 1
parent := &maxVersion
if maxVersion == 0 { parent = nil }

// 改为
nextVersion := maxVersion + 1
var parent *int
if req.BaseVersion != nil {
    // 验证 baseVersion 存在
    if _, err := h.Versions.GetVersion(ctx, formulaID, *req.BaseVersion); err != nil {
        return 404
    }
    parent = req.BaseVersion
} else if maxVersion > 0 {
    parent = &maxVersion
}
```

**注意**：baseVersion 是 archived 也允许；归档不影响只读访问。新版本始终是 draft。

### 3.3 不动什么

- `GET /formulas/:id/versions/:n` 已经支持任意状态读取，不动
- 状态机本身（draft/published/archived 转换）不动
- max version 计算逻辑不变，新 fork 仍然是 max+1

## 4. 前端设计

### 4.1 `VersionsPage.tsx` Edit 按钮

每行加一个「编辑」按钮（i18n key `version.edit`）：

```tsx
<button onClick={() => handleEdit(v)}>
  {t('version.edit')}
</button>
```

```tsx
function handleEdit(v: FormulaVersion) {
  if (v.state === 'archived') {
    // 弹确认
    if (!window.confirm(t('version.editArchivedConfirm', { version: v.version, next: maxVersion + 1 }))) {
      return
    }
  }
  // 编辑：navigate 到编辑页带 baseVersion 参数
  navigate(`/formulas/${id}?baseVersion=${v.version}`)
}
```

> 用 `window.confirm` 是务实选择——已经是阻塞式确认，不引入新的 modal 组件。

### 4.2 `FormulaEditorPage.tsx` 加载逻辑

```tsx
const [searchParams] = useSearchParams()
const baseVersion = searchParams.get('baseVersion')  // string | null

// 加载 graph
useEffect(() => {
  if (baseVersion) {
    // fork 模式：加载指定版本的 graph
    api.get(`/formulas/${id}/versions/${baseVersion}`).then(v => {
      setNodes(v.graph.nodes)
      setEdges(v.graph.edges)
      setForkSource({ version: v.version, state: v.state })
    })
  } else {
    // 默认：加载最新版本（保持当前行为）
  }
}, [id, baseVersion])
```

### 4.3 Fork mode banner

编辑器顶部一个常驻横幅：

```tsx
{forkSource && (
  <div className="bg-amber-50 border-l-4 border-amber-400 p-4 mb-4">
    <p className="text-amber-800">
      {t('editor.forkModeBanner', {
        version: forkSource.version,
        state: t(`version.${forkSource.state}`),  // i18n 标签
        next: maxVersion + 1,
      })}
    </p>
  </div>
)}
```

文案：「正在基于 v3 (archived) 编辑。保存后会生成新版本 v6，原 v3 保持归档不变。」

### 4.4 Save 时带上 baseVersion

```tsx
async function handleSave() {
  const body: any = { graph, changeNote }
  if (baseVersion) {
    body.baseVersion = parseInt(baseVersion, 10)
  }
  const newVersion = await api.post<FormulaVersion>(`/formulas/${id}/versions`, body)
  // 保存成功后清掉 baseVersion，进入"编辑新 draft"模式
  navigate(`/formulas/${id}`, { replace: true })
}
```

### 4.5 i18n keys（en/zh/ja）

```jsonc
"version": {
  // 已有 ...
  "edit": "Edit" / "编辑" / "編集",
  "editArchivedConfirm": "You are about to fork the archived version v{{version}}. Saving will create a new version v{{next}}; the original archived version will remain unchanged. Continue?" / ... / ...,
  "draft": "Draft" / "草稿" / "下書き",
  "published": "Published" / "已发布" / "公開済み",
  "archived": "Archived" / "已归档" / "アーカイブ済み"
},
"editor": {
  // 已有 ...
  "forkModeBanner": "Editing fork from {{state}} v{{version}}. Saving will create new version v{{next}}; the original {{state}} version will remain unchanged." / ... / ...
}
```

## 5. 涉及文件

### 后端
- `backend/internal/api/dto.go`（或 version_handler.go）— `CreateVersionRequest` 加 `BaseVersion`
- `backend/internal/api/version_handler.go` — Create logic 用 baseVersion 设 parent
- `backend/internal/api/version_handler_test.go` 或新建 — 验证 fork 行为

### 前端
- `frontend/src/components/version/VersionsPage.tsx` — Edit 按钮 + handleEdit
- `frontend/src/components/editor/FormulaEditorPage.tsx` — 读 baseVersion + fork 加载 + banner + save 时带参
- `frontend/src/i18n/locales/{en,zh,ja}.json` — 5+ 新 key

## 6. 风险与边界

| 场景 | 处理 |
|---|---|
| 已存在 draft v6，用户基于 archived v3 编辑 | 系统创建 v7（next = max+1），用户的 fork 落到 v7。Banner 应该正确显示 next=v7。 |
| 多 tab 同时编辑 | 各自独立，互不干扰。Save 后各自取到的 next 可能竞争——后保存的胜出，得到更大的 version 号。 |
| baseVersion 不存在 | 后端 404 |
| baseVersion 是当前最新版本 | 等价于普通编辑——前端可以选择不显示 banner，但代码上保持一致行为也 OK |
| 用户在 fork mode 想退出（不保存） | navigate 回 `/formulas/${id}` 即可（去掉 query param） |
| 老的 changeNote 自动填充 | 建议 fork 模式下默认 changeNote 是 `"Forked from v{baseVersion}"`，用户可改 |
