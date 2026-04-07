# 005 - 大批量测试功能

**Status**: in-progress
**Created**: 2026-04-07

## 需求

提供一个批量测试页面：
- 选择公式（含版本）
- 上传测试数据集（JSON 或 CSV 格式），每条数据包含输入参数和期待结果
- 批量对该公式执行计算，逐一与期待结果对比
- 在页面上显示测试结果（通过/失败、实际值 vs 期待值、执行耗时）
- 汇总统计（总用例数、通过数、失败数、通过率）

## 设计

### 数据格式

**JSON 上传格式：**
```json
[
  {
    "inputs": { "deathBenefit": "1000000", "expectedDeaths": "5", "policyCount": "1000" },
    "expected": { "n6": "5000" },
    "label": "标准场景"
  }
]
```

**CSV 上传格式：** 首行为列名，`expected_<outputId>` 前缀标识期待值列，其余列为输入。
```
label,deathBenefit,expectedDeaths,policyCount,expected_n6
标准场景,1000000,5,1000,5000
高风险场景,2000000,8,500,32000
```

### 后端

新增端点 `POST /api/v1/calculate/batch-test`：
- 请求体：`{ formulaId, version?, tolerance?, cases: [{inputs, expected, label?}] }`
- `tolerance`：小数比较容差（默认 "0"，可配置如 "0.01" 表示 1% 容差）
- 响应：`{ summary: {total,passed,failed}, results: [{label,inputs,expected,actual,pass,diff,executionTimeMs,cacheHit}] }`
- 对比逻辑：`|actual - expected| <= tolerance * |expected|`（相对容差）；expected 为 "0" 时用绝对容差

### 前端

- 新建 `components/shared/BatchTestPage.tsx`
  - 公式选择器（下拉，仅显示有 published 版本的公式）
  - 版本选择器（默认最新 published）
  - 容差输入（默认 0%）
  - 上传区：支持 JSON / CSV 文件，或直接粘贴 JSON
  - 「运行测试」按钮
  - 结果汇总卡片（total / passed / failed / 通过率）
  - 结果明细表格：序号、label、pass/fail、实际值、期待值、偏差、耗时
- `App.tsx`：添加 `/batch-test` 路由
- `Navbar.tsx`：为 editor+ 角色添加导航入口
- i18n：zh/en/ja 添加 `batchTest.*` keys

## TODO

- [x] 创建任务文件
- [ ] 后端：DTO（BatchTestRequest / BatchTestResponse）
- [ ] 后端：batch-test handler（含容差对比逻辑）
- [ ] 后端：router 注册 POST /calculate/batch-test
- [ ] 前端：BatchTestPage.tsx（公式选择 + 文件上传 + 结果表格）
- [ ] 前端：CSV 解析工具函数
- [ ] 前端：App.tsx 路由
- [ ] 前端：Navbar 入口
- [ ] 前端：i18n（zh/en/ja）
- [ ] codex review + fix
- [ ] 提交
