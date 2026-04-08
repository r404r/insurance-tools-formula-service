# Task #018: LaTeX 公式输入功能测试套件

## Status: done

## 需求

针对 Task #017 实现的 LaTeX 表达式输入创建公式功能，设计覆盖所有 LaTeX 语法的 20 组测试 case，进行以下验证：

1. **转换正确性**：输入 LaTeX → 转化为图形公式 → 截图验证视觉呈现
2. **计算正确性**：每组 case 设计 10 套入参，调用计算 API，验证计算结果
3. **测试报告**：汇总为 Markdown 文档
4. **可复用脚本**：测试用例写成可多次运行的自动化脚本
5. **缺陷修复**：发现问题时定位根因、修复、codex review、回归测试

## 设计

### 测试分层

| 层 | 工具 | 文件 | 验证目标 |
|----|------|------|----------|
| Unit | vitest | `frontend/src/utils/__tests__/latexToFormula.test.ts` | LaTeX → formula text 转换逻辑 |
| E2E / 截图 | Playwright | `tests/018-latex-suite/latex-e2e.spec.ts` | 视觉图形公式 + API 计算结果 |
| 报告 | 生成脚本 | `tests/reports/018-latex-test-report.md` | 汇总结果 |

### 20 组测试 Case 语法覆盖点

| # | LaTeX 语法 | 覆盖点 |
|---|-----------|--------|
| 01 | `\mathrm{age} + \mathrm{base\_rate}` | 变量标识符、下划线变量名、加法 |
| 02 | `\mathrm{premium} - \mathrm{discount}` | 减法 |
| 03 | `\mathrm{sum\_insured} \cdot \mathrm{rate}` | `\cdot` → `*` |
| 04 | `\mathrm{principal} \times \mathrm{factor}` | `\times` → `*` |
| 05 | `\frac{\mathrm{annual}}{12}` | `\frac` → 除法 |
| 06 | `\mathrm{x}^{2}` | `^{...}` → 幂运算 |
| 07 | `e^{\mathrm{r}}` | `e^{...}` → `exp(...)` |
| 08 | `\sqrt{\mathrm{x}}` | `\sqrt` → `sqrt(...)` |
| 09 | `\left|\mathrm{a} - \mathrm{b}\right|` | 绝对值 `\left|...\right|` → `abs(...)` |
| 10 | `\ln\left(\mathrm{x}\right)` | `\ln\left(...\right)` → `ln(...)` |
| 11 | `\operatorname{max}\left(\mathrm{a}, \mathrm{b}\right)` | `\operatorname` → 自定义函数 |
| 12 | `\operatorname{min}\left(\mathrm{lo}, \mathrm{hi}\right)` | `\operatorname` min 函数 |
| 13 | `\left(\mathrm{a} + \mathrm{b}\right) \cdot \mathrm{c}` | `\left(...\right)` 括号分组 |
| 14 | `\mathrm{age} \ge 18` | `\ge` → `>=` |
| 15 | `\mathrm{risk} \le 0.5` | `\le` → `<=` |
| 16 | `\mathrm{status} \ne 0` | `\ne` → `!=` |
| 17 | `\mathrm{x} \geq \mathrm{y}` | `\geq` (等价形式) |
| 18 | `\begin{cases}...\end{cases}` 简单条件 | cases → `if ... then ... else ...` |
| 19 | `\begin{cases}` 嵌套条件 | 嵌套 cases |
| 20 | 复合公式：`\frac{\mathrm{sum\_insured} \cdot \mathrm{rate}}{1000}` | 多运算符组合 |

### 涉及文件

- `frontend/src/utils/__tests__/latexToFormula.test.ts`（新建）
- `tests/018-latex-suite/latex-e2e.spec.ts`（新建）
- `tests/018-latex-suite/playwright.config.ts`（新建）
- `tests/018-latex-suite/package.json`（新建）
- `tests/018-latex-suite/run.sh`（新建）
- `tests/screenshots/018/`（截图输出目录）
- `tests/reports/018-latex-test-report.md`（测试报告，自动生成）

## TODO

- [x] 创建任务文件（本文件）
- [x] 更新 backlog.md
- [ ] 实现 vitest 单元测试（20 case，验证 LaTeX → formula text 转换）
- [ ] 配置 Playwright 测试目录（package.json + playwright.config.ts）
- [ ] 实现 E2E 测试（截图 + API 计算验证）
- [ ] 运行单元测试，修复发现的 bug
- [ ] 运行 E2E 测试，保存截图
- [ ] 生成测试报告 markdown
- [ ] 若有代码修复：codex review + 回归测试
- [ ] commit

## 完成标准

- [ ] 20 组 vitest 单元测试全部通过
- [ ] 每组 case 的视觉截图保存到 `tests/screenshots/018/`
- [ ] 每组 case 的 API 计算结果与预期值一致
- [ ] `tests/reports/018-latex-test-report.md` 文档已生成
- [ ] commit + codex review
