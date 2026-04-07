# Task #009: 保险领域公式模板

## Status: done

## 需求

当前数据库里有若干 seed 公式（life 5 个、property 1 个、auto 1 个），但用户无法在 UI 中以「从模板创建」的方式快速启动新公式。需要：

1. 后端提供 `/api/v1/templates` 接口，返回预置模板列表（代码内置，不依赖 DB）
2. 前端「公式列表」页面新增「从模板创建」入口，弹出模板画廊
3. 模板画廊支持按领域（life/property/auto）筛选，选中后一键预填表单创建新公式
4. 三个领域各至少补充 2 个新模板（目前 property 和 auto 偏少）

## 设计

### 后端

**`GET /api/v1/templates`**（无需认证）

返回：
```json
{
  "templates": [
    {
      "id": "tpl-life-net-premium",
      "domain": "life",
      "name": "寿险净保费",
      "description": "...",
      "graph": { "nodes": [...], "edges": [...] }
    }
  ]
}
```

- `internal/api/template_handler.go`（新）— 静态模板数据 + handler
- `internal/api/router.go` — 注册 `GET /templates`（public 路由）

不存 DB，代码内置 template 定义，版本迭代改代码即可。

### 模板列表（共 9 个）

**Life（3 个）：**
- `tpl-life-net-premium` — 寿险净保费：`netPremium = sumAssured × mortalityRate × discountFactor`
- `tpl-life-term-risk` — 定期寿险风险保费：`premium = sumAssured × qx × (1 + loadingRatio)`
- `tpl-life-annuity` — 年金现值：`annuityPV = annualBenefit × annuityFactor`

**Property（3 个）：**
- `tpl-property-basic` — 财产险基础保费：`premium = insuredValue × baseRate × occupancyFactor`
- `tpl-property-home` — 家财险保费：`premium = buildingValue × buildingRate + contentsValue × contentsRate`
- `tpl-property-engineering` — 工程险保费：`premium = contractAmount × baseRate × projectFactor × durationFactor`

**Auto（3 个）：**
- `tpl-auto-commercial` — 车险商业保费：`premium = basePremium × vehicleFactor × driverFactor × ncdDiscount`
- `tpl-auto-compulsory` — 交强险：`premium = basePremium × vehicleTypeFactor`
- `tpl-auto-ubi` — UBI 里程险：`premium = basePremium × mileageFactor × drivingScoreFactor`

### 前端

- `src/api/templates.ts`（新）— `getTemplates(): Promise<Template[]>`
- `src/types/formula.ts` — 追加 `Template` 类型
- `src/components/shared/TemplateGallery.tsx`（新）— 模板画廊弹窗
  - 顶部 domain 切换 tab（All / 寿险 / 财产险 / 车险）
  - 模板卡片：标题、领域徽章、描述、节点数
  - 「使用此模板」按钮 → 打开 CreateFormulaModal，预填 name/domain/graph
- `src/components/shared/FormulaList.tsx` — 追加「从模板创建」按钮（在「新建公式」旁边）
- i18n zh/en/ja 追加 `template.*` 键

## 涉及文件

- `backend/internal/api/template_handler.go`（新）
- `backend/internal/api/router.go`
- `frontend/src/types/formula.ts`
- `frontend/src/api/templates.ts`（新）
- `frontend/src/components/shared/TemplateGallery.tsx`（新）
- `frontend/src/components/shared/FormulaList.tsx`
- `frontend/src/i18n/locales/zh.json`, `en.json`, `ja.json`

## TODO

- [x] 创建任务文件
- [x] 后端：template_handler.go（9 个模板数据 + GET /templates）
- [x] 后端：router.go 注册路由
- [x] 后端：go test ./... 通过
- [x] 前端：types/formula.ts 追加 Template 类型
- [x] 前端：api/templates.ts
- [x] 前端：TemplateGallery.tsx
- [x] 前端：FormulaList.tsx 追加入口按钮
- [x] i18n（zh/en/ja）
- [x] codex review + fix P1/P2
- [x] 提交

## 完成标准

- [x] 功能正常（从模板创建公式，图谱正确出现在编辑器）
- [x] 测试通过
- [x] commit + codex review
