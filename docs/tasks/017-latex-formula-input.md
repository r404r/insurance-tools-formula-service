# Task #017: LaTeX 表达式输入创建公式

## Status: done

## 需求

在公式编辑器文本模式下，追加 LaTeX 输入方式。用户粘贴或输入 LaTeX 公式，
系统自动转换成内部公式文本（text expression），再经由现有 parse 流程生成 DAG。

## 设计

### 技术方案

- 反向转换：formulaTextToLatex 的逆向——实现 `latexToFormulaText(latex) → formulaText`
- 覆盖 formulaTextToLatex 能生成的所有 LaTeX 构造（frac, sqrt, operatorname, left|, ln, exp, cases, aligned 等），以及用户常手写的等价形式（\times, \geq 等）
- 转换完成后直接复用现有 parse API → React Flow 渲染流程，无需后端改动

### UI 变更

TextEditor 新增 "LaTeX" 模式 tab（"Text" tab 保留）：
- LaTeX 模式：左半 LaTeX textarea，右半实时 KaTeX 渲染 + 下方转换结果文本预览
- Apply 按钮：将转换后的 formulaText 传给现有 onChange → parseFormula 流程
- 转换出错时显示错误提示而不是崩溃

### 支持的 LaTeX 语法

| LaTeX | 转换为 |
|-------|--------|
| `\mathrm{name}` | `name` |
| `\frac{a}{b}` | `(a) / (b)` |
| `a^{b}` | `a ^ (b)` |
| `e^{x}` | `exp(x)` |
| `\sqrt{x}` | `sqrt(x)` |
| `\left\|x\right\|` | `abs(x)` |
| `\ln\left(x\right)` | `ln(x)` |
| `\operatorname{fn}\left(args\right)` | `fn(args)` |
| `\left(x\right)` | `(x)` |
| `\cdot`, `\times` | `*` |
| `\ge`, `\geq` | `>=` |
| `\le`, `\leq` | `<=` |
| `\ne`, `\neq` | `!=` |
| `\begin{cases}...\end{cases}` | `if cond then a else b` |
| `\begin{aligned}...\end{aligned}` | 多行公式 |

### 涉及文件

- `frontend/src/utils/latexToFormula.ts`（新建）
- `frontend/src/components/editor/TextEditor.tsx`（修改）
- `frontend/src/i18n/locales/{zh,en,ja}.json`（新增 i18n key）

## TODO

- [x] 创建任务文件
- [x] `latexToFormula.ts`：LaTeX → formula text 转换器
- [x] `TextEditor.tsx`：追加 LaTeX tab
- [x] i18n 三个 locale 新增 key
- [x] tsc --noEmit 通过
- [x] codex review + fix P1/P2
- [x] 提交

## Codex Review

- P2 fix: `transformCases()` 改用 `findTopLevelDoubleBackslash()` 处理嵌套 conditional
- P2 fix: `\mathrm{...}` 提取时 unescape `\_` → `_`，修复下划线变量名
- P2 fix: `\sqrt[n]{...}` 不再静默转为 sqrt，改为抛出明确错误

## 完成标准

- [x] 用户输入 LaTeX，能正确转换为 formula text 并 parse 成 DAG
- [x] tsc 通过
- [x] commit
