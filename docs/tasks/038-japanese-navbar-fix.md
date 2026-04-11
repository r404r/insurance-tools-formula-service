# Task #038: 日语 Navbar 换行不美观修复

## Status: done

## 需求

切换到日语时，顶部 Navbar 所有菜单项都换行（`計算式管` / `理`、
`ルックアップテーブ` / `ル`、`分類管` / `理` 等），App title
`保険計算式エンジ` / `ン` 也裂成两行，视觉上非常乱。

用户截图确认了问题：每一条菜单都在换行。

## 分析

根因两层叠加：

### 1. 翻译不简洁（内容问题 — 主要原因）

当前 JA `nav.*` 词条每条都带「～管理」「～設定」后缀，相比 EN
（`Formulas`/`Tables`/`Users`/`Settings`，全部是光秃的名词）和 ZH
（字符窄）显得过长。其中 `ルックアップテーブル` 是 10 个全角宽的
片假名长串，是最严重的一个。

日本产品实际 UX 里 navbar 普遍用简洁名词（サイボウズ、Notion 日语版
都是 `ユーザー`/`設定`，没人加「管理」），当前的翻译更像从中文硬翻
过去的。

### 2. Navbar 没有 `whitespace-nowrap`（样式问题 — 次要）

`Navbar.tsx` 的 `<Link>` 元素没有阻止换行的 class，所以哪怕翻译已经
短了，只要 flex 拥挤浏览器就会按字符断行。

## 设计

采用"方案 A + 用户选择"：修翻译为主，CSS nowrap 作安全网。

### i18n 改动（只改 ja.json）

| Key | Before | After |
|---|---|---|
| `app.title` | `保険計算式エンジン` | 不变（codex P2 修复后保留「保険」保证三语语义对等） |
| `nav.formulas` | `計算式管理` | `計算式` |
| `nav.tables` | `ルックアップテーブル` | `参照テーブル` |
| `nav.categories` | `分類管理` | `分類` |
| `nav.users` | `ユーザー管理` | `ユーザー` |
| `nav.cache` | `キャッシュ管理` | `キャッシュ` |
| `nav.batchTest` | `バッチテスト` | 不变（已经简洁） |
| `nav.adminSettings` | `システム設定` | `設定` |
| `nav.logout` | `ログアウト` | 不变（在下拉菜单里，不在 navbar） |

> codex review 指出 EN `Insurance Formula Engine` / ZH `保险公式计算引擎` 都保留
> 「保险/insurance」语义，若 JA 缩成 `計算式エンジン` 会丢掉保险语境，三语不对称。
> 实测加了 `whitespace-nowrap` + 缩短的 nav 标签后，原始长度 `保険計算式エンジン`
> 在 1280 宽度下单行完整显示毫无压力，所以恢复原标题。

ZH 和 EN 不动（用户确认只聚焦 JA）。

### Navbar.tsx 改动

给所有 `<Link>` 和 `<button>` 加 `whitespace-nowrap`：

- 6 个 nav link（formulas / tables / batch-test / categories / users / settings）
- App title `<Link>`
- 语言切换 `<button>`
- 用户菜单 `<button>`

其它 className 一律不动，布局不变。

## 涉及文件

- `frontend/src/i18n/locales/ja.json` — 9 个 key 修改
- `frontend/src/components/shared/Navbar.tsx` — 加 `whitespace-nowrap`

## TODO

- [x] 用户确认方案 A
- [x] 用户确认子选项（`参照テーブル` / 不动 ZH；app.title 经 codex review 后恢复原样）
- [x] 修改 `ja.json`
- [x] 修改 `Navbar.tsx`（app title + 6 nav Link + lang button + user button 共 9 处加 `whitespace-nowrap`）
- [x] `cd frontend && npm run build` 通过
- [x] 浏览器冒烟：切到 JA，navbar 单行显示（`tests/screenshots/038/17-nav-ja-full-title.png`）
- [x] 截图对比（前：用户原始图；后：17-nav-ja-full-title.png）
- [x] codex review → P2 修复（恢复 `保険` 前缀）→ commit

## 完成标准

- [x] 切到日语后 navbar 每个菜单项一行显示，无换行
- [x] App title 一行显示（`保険計算式エンジン` 完整）
- [x] EN / ZH 切换后无副作用（只改了 ja.json 和 Navbar CSS，EN/ZH 文本未动，nowrap 对所有语言都是安全的）
- [x] 语义保真：`参照テーブル` 保留 lookup 含义；标题恢复后保留「保険」语境
