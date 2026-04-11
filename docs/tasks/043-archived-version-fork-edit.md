# Task #043: 编辑已归档版本生成新版本

## Status: done

## 需求

实现 [`docs/specs/006-archived-version-fork-edit.md`](../specs/006-archived-version-fork-edit.md)：

1. 版本历史每行加「编辑」按钮，所有状态（draft/published/archived）都可见
2. 点击 archived 版本的编辑按钮时弹一次确认对话框
3. 编辑器接受 `?baseVersion=N` URL 参数，加载该版本的 graph
4. 编辑器顶部显示 fork mode banner
5. 保存时后端创建新 draft，`ParentVersion = baseVersion`，原版本不变

## TODO

### 后端
- [x] `CreateVersionRequest` DTO 加 `BaseVersion *int`
- [x] `version_handler.go` Create 用 BaseVersion 设 ParentVersion，验证 baseVersion 存在
- [x] `auth.WithClaims` helper（测试用，注入 claims 不走 middleware）
- [x] 单元测试 5 个：default parent、fork from archived、fork from published、missing base 404、fork stamps updater meta

### 前端
- [x] `VersionsPage.tsx` 每行加「编辑」按钮 + handleEdit
- [x] archived 状态的 handleEdit 弹 window.confirm
- [x] **codex round 1 P1**：每个状态都用 `?baseVersion=N` URL（支持给 published 也 fork）
- [x] 加 i18n 三语 keys（`version.edit` / `version.editArchivedConfirm` / `editor.forkModeBanner`）
- [x] `FormulaEditorPage.tsx` 读 `?baseVersion=N`，加载该版本的 graph
- [x] **codex round 2 P2**：isForkMode = baseVersion < maxVersion（避免编辑 latest 时显示 fork banner）
- [x] Fork mode banner（顶部 amber 横幅）
- [x] Save 时带上 baseVersion
- [x] **codex round 3 P1**：保存成功后无条件清除 baseVersion query param（避免 maxVersion 刚 bump 后倒退到旧版）

### 验证
- [x] `go vet ./... && go test ./... -race` 通过
- [x] `npm run build` 通过
- [x] codex review 四轮 → P1×2 + P2×1 全部修复 → LGTM
- [x] 移到 backlog 已完成

## 完成标准

- [ ] Archived 版本可以通过编辑按钮编辑，保存后生成新 draft
- [ ] 原 archived 版本保持不变
- [ ] 新 draft 的 ParentVersion 指向 baseVersion
- [ ] Fork banner 正确显示版本号
- [ ] race detector + 单元测试全绿
- [ ] codex review LGTM
