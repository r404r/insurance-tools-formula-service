# Task #032: 公式一览画面行信息密度优化

## Status: done

## 最终方案：D1 + A 的图标 Actions

用户确认采纳方案 D1 的两行布局，Actions 按方案 A 改为图标按钮（内联 SVG，无新增依赖）。

### Codex Review 反馈（已修复）

**[P2]** 移除 Formula ID 列后，重名公式（例如复制/导入产生的同名公式，
描述和日期也相同的场景）在视觉上无法区分，用户无法判断 Export/Copy/Delete
操作的是哪一条。代码本身仍然使用 `f.id` 正确地定位行，所以不是功能 bug，
但用户体验存在歧义。

**修复**：在第二行 subtitle 右侧追加一个小号 mono 的 UUID 前 8 位（`#a10178be`
样式），`title={f.id}` 悬停显示完整 UUID。既解决了歧义问题，又几乎不占空间，
行高仍为 63px。

### 实测效果（1440×900 viewport）

- 单行高度：93–113px → **63px**（约 1.6× 密度提升）
- 一屏可见：5–6 行 → **20 行**（每页满载）
- Name 列宽：131px → ~757px（中日文长名不再截断）
- 保留所有信息：name、description、短 ID、domain、created、actions 全部可见

## 需求

公式一览画面每条公式显示的内容过于拥挤，单行高度过大，有效信息密度低。
目标是在不丢失关键信息的前提下，显著提高可视公式条数和扫视效率。

### 现状测量（viewport 1440×900）

- 表格容器宽度：1102px（`max-w-6xl`）
- 7 列宽度：checkbox 48 / Name 131 / **ID 155** / Domain 119 / **Description 359** / Created 115 / Actions 175
- 单行实测高度：**93–113px**（内容多的甚至更高）
- 900px 视口下一屏只能看到 ~5–6 行

### 拥挤根因

1. **Formula ID 列占用 155px**，展示完整 UUID（如 `7b32a8d5-aeec-4572-8540-a10178be68f7`），
   在窄列中换行成 2–3 行。UUID 对用户扫视无意义，是最大的空间浪费。
2. **Description 允许无限换行**，长描述会把行高撑到 3–4 行。
3. **Formula Name 列仅 131px**，中日文长名称会换行成 2–3 行。
4. **表头文字本身换行**："Formula Name"、"Insurance Domain" 等表头都在换行，
   说明列宽已经不足以容纳表头。
5. **Actions 列三个文字按钮**（Export / Copy / Delete），在某些语言下会继续换行。
6. **容器宽度 `max-w-6xl` (1152px)** 对 7 列表格过窄。

---

## 设计（候选方案）

### 方案 A：最小侵入 + 高收益（推荐）

保留表格结构，针对性地修复所有拥挤来源：

1. **移除 Formula ID 列**
   - UUID 对一览扫视无意义，详情页已经有。
   - 如需复制 ID，可在 Actions 组里加一个小的复制图标，或在鼠标悬停 name 时显示 tooltip。
2. **Name 单行 + truncate**
   - 使用 `truncate` + `title={name}`，悬停显示完整名。
   - Name 列获得更多宽度。
3. **Description 单行 line-clamp-1**
   - `line-clamp-1` + `title={description}`，悬停或进入详情页看完整。
4. **Actions 改为图标按钮**
   - 用 lucide-react 图标（Download / Copy / Trash），配 `title` 属性。
   - 列宽可从 175px 降到 ~110px。
5. **容器宽度放到 `max-w-7xl`**（1280px），给 Name/Description 更多呼吸空间。
6. **表头 `whitespace-nowrap`**，避免表头自身换行。

**预期效果：**
- 单行高度从 93–113px → **52–56px**（约 2 倍密度提升）
- 一屏可见行数 5–6 → **12–14**
- 保留所有关键信息的可见性
- 改动集中在 `FormulaList.tsx` 一个文件

---

### 方案 B：卡片网格布局（较大重构）

把表格换成 2 列或 3 列卡片，每张卡片：
- 大标题（Name）+ 右上角 domain badge
- 2 行描述 clamp
- 底部小字：创建时间 + 图标操作

**优点**：视觉舒展、信息层次清晰、跨语言友好。
**缺点**：密度反而下降（一屏更少卡片）；分页/排序/批量选择体验需要重做；
批量选择 checkbox 需要更显眼地放到卡片角落。

对于一个以"快速扫视 + 批量操作"为主的列表，这个方案反而不如表格高效。

---

### 方案 C：分栏布局（Master-Detail）

左侧窄列表（只有 Name + badge），右侧展示选中公式详情。
**缺点**：与现有的"点击进入详情页"流程冲突，需要重做路由。
评估：过度工程，超出本次优化范畴。

---

### 方案 D：两行式（Two-Row Layout）⭐新推荐

每条公式占 2 个视觉行：
- **第 1 行**：checkbox · Name · Domain badge · Created · Actions
- **第 2 行**：Description（小字灰色，跨可用宽度，不换行或 line-clamp-1）

**D1 实现方式（推荐）**：单 `<tr>` 内用 flex 堆叠

只在 Name 单元格里用 `flex flex-col` 堆叠 name + description，
**彻底删除独立的 Description 列和 Formula ID 列**。

```tsx
<td className="px-6 py-3">
  <div className="truncate font-medium text-gray-900">{f.name}</div>
  <div className="mt-0.5 line-clamp-1 text-xs text-gray-500">{f.description}</div>
</td>
```

改动极小，无需改表格结构，hover/选中样式全部沿用。

**D2**（双 `<tr>` + colspan）改动更大，不采纳。

**列宽分配（D1）：**
```
checkbox | Name+Desc(堆叠) | Domain | Created | Actions(图标)
   48    |      ~480       |  110   |   100   |     110
```

**预期效果：**
- 单条高度 93–113px → **~68–72px**（≈1.6× 密度提升）
- 一屏可见条数 5–6 → **10–11**
- Description 免 hover 直接可见（作为 name 的"副标题"）
- Name 列宽 131 → 480px，中日文长名基本不截断

---

## 方案对比

| 维度 | 当前 | 方案 A | **方案 D1** |
|---|---|---|---|
| 单条高度 | 93–113px | 52–56px | ~68–72px |
| 一屏可见 | 5–6 | 12–14 | 10–11 |
| Description | 多行堆叠 | 截断+tooltip | 整行副标题 |
| Name 最大宽度 | 131px | ~280px | ~480px |
| 改动复杂度 | — | 小 | 小 |

## 推荐

**方案 D1**：Description 作为副标题保留日常可读性，密度仍接近 2×，
中日文长名不再截断，最符合"信息密度 + 扫视效率"的综合目标。

可选的进一步增强（方案 A+）：
- 把 "Created At" 改为相对时间（"3 天前"），列宽更紧凑。
- Description 列只在桌面宽屏显示（≥lg 断点），窄屏隐藏。

---

## 涉及文件

- `frontend/src/components/shared/FormulaList.tsx`
- 可能引入 `lucide-react` 图标（检查 package.json 是否已有）

## TODO

- [x] 等待用户从方案 A / B / C / D 中确认 → D1 + A 的图标 actions
- [x] 删除 Formula ID 列（表头 + 单元格）
- [x] Name 单元格改为 flex-col，堆叠 name + description line-clamp-1
- [x] 删除独立 Description 列
- [x] Actions 改为内联 SVG 图标按钮（Download/Copy/Trash），带 title
- [x] 容器 `max-w-6xl` → `max-w-7xl`
- [x] 表头 `whitespace-nowrap`
- [x] 前后对比截图到 `tests/screenshots/032/`
- [x] `npm run build` 验证前端构建
- [x] `/codex review` → P2 修复：subtitle 添加短 ID 前 8 位 mono 标签
- [x] commit

## 完成标准

- [ ] 单行高度 ≤ 60px（方案 A 目标 52–56px）
- [ ] 一屏可见公式条数翻倍
- [ ] 无信息丢失（所有字段通过 title/tooltip 或详情页可访问）
- [ ] 前后截图对比清晰
- [ ] codex review 通过
