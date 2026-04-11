# Task #047: Seed 数据提取到外部脚本（API-driven）

## Status: in-progress

## 需求

`backend/cmd/server/main.go` 当前 1640 行，其中约 1200 行是 `seed()` 函数硬编码的预置数据：

- 24 个公式（`seedFormula(...)` 调用 + 手写 `domain.FormulaGraph{Nodes:..., Edges:...}` 字面量）
- 2 个 lookup table（`日本標準生命表2007（簡易版）` + `claims_triangle_sample`）
- 默认 admin user
- 3 个分类（life / property / auto）

**问题**：
1. 每加一个 seed 公式 = 改 Go 代码 + 重新 build 镜像 + 重启容器，循环慢（task #045 一次加 17 个公式即暴露此瓶颈）
2. 手写 graph 字面量容易出错（task #045 1/24 法 inverted bug 即由此产生，codex 多轮 review 才发现）
3. main.go 已经过度膨胀，单文件难以审查
4. 不走 API 路径 = 不经过用户使用时同样的代码路径（validator、UpdateMeta、auth 等）

**目标**：
- 把 seed 公式 + 表数据从 main.go 提取成独立的 JSON bundle 文件
- 通过专用的初始化脚本调用 API 进行 seed
- 保留 admin/category bootstrap 在 main.go（API 无法 bootstrap 自身的 admin）
- 整个改造分两阶段（阶段 1 = 低风险迁移，阶段 2 = 完美化）

## 设计

### Bundle 格式

复用现有的 `ExportBundle`（task #029 引入，`api/formula_handler.go:368`）：

```json
{
  "version": "1.0",
  "exportedAt": "2026-04-11T22:00:00Z",
  "formulas": [
    {
      "sourceId": "寿险净保费计算",
      "sourceVersion": 1,
      "name": "寿险净保费计算",
      "domain": "life",
      "description": "...",
      "graph": { "nodes": [...], "edges": [...], "outputs": [...], "layout": {...} }
    }
  ]
}
```

**关键约定**：seed bundle 的 `sourceId` 不再是真 UUID，而是一个**逻辑名（== name）**，作为跨公式引用的稳定 key。

### 跨引用问题（关键技术点）

当前 seed 数据存在两类跨引用：

1. **公式 → 公式**（sub-formula）：例如 `定期保険一時払純保険料` 引用 `body2ID`（死亡給付PV項）
2. **公式 → 表**（table lookup / aggregate）：例如 `日本生命保険 死亡率 qx` 引用 `tableID`（生命表），LDF 引用 `claims_triangle_sample`

bundle 里 graph 节点的 `formulaId` / `tableId` 当前是真 UUID，但导入后 backend 会生成全新 UUID，引用就断了。

**解决方案：占位符 + 客户端替换**

- bundle 里跨引用使用占位符：`{{formula:生存率因子 1-qx}}` / `{{table:claims_triangle_sample}}`
- 脚本运行时：
  1. 先 import 所有表 → 建 `tableName → realID` map
  2. 按依赖序 import 公式 → 每次 import 之前对 graph 字符串做占位符替换 → import → 把新返回的 ID 加入 `formulaName → realID` map
- 依赖序由 bundle 文件名前缀控制（`010-...json` < `020-...json`，例如 sub-formula body 必须排在引用它的公式之前）

### 目录结构

```
backend/seed/
├── README.md              # 如何添加新 seed、占位符语法、依赖序约定
├── tables/                # lookup table 定义
│   ├── 010-life-table-2007.json
│   └── 020-claims-triangle-sample.json
├── formulas/              # 公式 bundle（每文件一个公式，便于 review）
│   ├── 010-life-net-premium.json
│   ├── 020-jp-life-equivalence.json
│   ├── ...
│   ├── 100-jp-survivor-factor-body.json     # body sub-formulas 排前面
│   ├── 110-jp-mortality-pv-body.json
│   ├── 200-jp-term-life-single-pay.json     # 引用 100/110
│   └── ...
└── runner/
    └── seed.go            # CLI 工具，cd seed/runner && go run .
```

> **为什么用 Go 写 runner 而不是 bash/node？** 项目本来就是 Go monorepo，加 bash 引入 jq/curl 依赖、加 node 引入 npm 依赖；Go runner 可以直接用 `net/http` + `encoding/json`，无新依赖，跨平台。Bin 编译产物可放进 backend Dockerfile。

### Runner 行为

```
Usage: seed-runner [--base-url URL] [--admin-user USER] [--admin-pass PASS] [--dry-run]

1. Login as admin → get JWT
2. Read seed/tables/*.json (sorted by filename)
   For each: GET /api/v1/tables?name=... → if not found, POST /api/v1/tables → record realID
3. Read seed/formulas/*.json (sorted by filename)
   For each:
     - Substitute {{formula:NAME}} / {{table:NAME}} placeholders using accumulated maps
     - GET /api/v1/formulas?name=... → if found, skip
     - POST /api/v1/formulas/import (single-formula bundle)
     - PATCH /api/v1/formulas/{id}/versions/1 with {state: "published"}
     - Record formulaName → realID
4. Print summary: created N, skipped M, errors K
```

**幂等性**：基于 name 检测，已存在则跳过；多次运行结果一致。

### main.go 改造

保留：
- `run()` 主函数
- admin user bootstrap（不能通过 API 自举）
- 3 个 default categories（极少变更，且 API 路径需要 admin 已存在）
- `seedFormulaNames` / `seedTableNames`（仍由 reset-seed handler 使用）— 但**值改为从 bundle 文件名/内容推导**

删除：
- 所有 `seedFormula(...)` 调用（从约 line 380 到 line 1430）
- 所有 graph 字面量
- helper：`nVar` / `nConst` / `nOp` / `nFn` / `nTableAgg` / `eEdge`（不再有调用方）
- `seedTable` 调用 + life table data + claims triangle data 大数组

预期：main.go 从 1640 行 → 约 440 行。

### Reset-seed handler

当前 `makeSeedResetHandler`（main.go:1586）通过 `seedNames` map 删除已知 seed 数据，然后调用 `seed()` 重新生成。改造后：

- `seedNames` 改为运行时从 `seed/formulas/*.json` 文件名或内容动态计算
- `seed()` 函数本身不再 seed 公式，只做 admin/category bootstrap → reset 时只删 seed 公式 + tables
- 重新 seed 由 runner 完成（reset handler 不直接调 runner，文档要求 admin 在 reset 后手动跑 runner，或：reset handler 通过子进程调 runner？— 本 task 第一阶段**不**做这层）

**第一阶段策略**：reset-seed 只负责清理（删除 seed 公式 + 表）。重新 seed 需要管理员手动跑 `seed-runner`。这是**功能微弱回退**，但简化了第一次落地的风险面。第二阶段可以加自动重 seed。

### Docker 集成

docker-compose.yml 加一个 one-shot service：

```yaml
seed-runner:
  profiles: ["seed"]
  build:
    context: ./backend
    dockerfile: Dockerfile.seed
  depends_on:
    backend-sqlite: { condition: service_started }   # 实际取决于活动 profile
  environment:
    - SEED_BASE_URL=http://backend:8080
    - SEED_ADMIN_USER=admin
    - SEED_ADMIN_PASS=admin123
  command: ["/seed-runner"]
```

使用：
```bash
docker compose --profile sqlite up -d              # 起服务
docker compose --profile sqlite --profile seed run --rm seed-runner   # 跑一次 seed
```

> Compose 的 multi-profile 联动需要验证；如果不行，回退到"docker compose exec backend-sqlite /seed-runner"或宿主机直接 `go run`。

### 阶段划分

**阶段 1（本 task 主体，必须完成）**

- ✅ Bundle 格式确定 + 占位符语法
- ✅ Runner 实现（Go，单文件，无外部依赖）
- ✅ 用当前的 `seed()` 跑一次 → 用 export API dump 24 个公式 → 手动加占位符 → 落盘到 `seed/formulas/`
- ✅ Tables 单独 dump → 落盘到 `seed/tables/`
- ✅ 删除 main.go 中的 seed 公式/表代码
- ✅ 调整 reset-seed handler（只清理）
- ✅ Dockerfile.seed + docker-compose.yml 集成
- ✅ README 文档：如何加新 seed、如何 reset、如何 docker 跑
- ✅ 端到端验证：fresh DB → docker up → seed-runner → 全部 24 个公式 + 2 个表存在且可计算

**阶段 2（本 task 第二轮，scope 内但可独立 commit）**

- ✅ Runner 加 `--dry-run`（解析 + 校验 bundle，不写）
- ✅ Runner 加 `--only NAME` 单条 seed
- ✅ bundle 按 domain 分子目录：`formulas/life/`, `formulas/property/`, `formulas/auto/`
- ✅ Reset handler 自动调子进程跑 seed-runner（可选；如果太复杂保留人工触发）
- ✅ 单元测试：runner 的占位符替换函数 + bundle 解析
- ✅ Codex review

## 涉及文件

**新增**：
- `backend/seed/README.md`
- `backend/seed/tables/*.json`（2 个表）
- `backend/seed/formulas/*.json`（24 个公式 bundle）
- `backend/seed/runner/main.go`（CLI）
- `backend/seed/runner/main_test.go`（占位符替换 + bundle 解析测试）
- `backend/Dockerfile.seed`
- `docs/tasks/047-seed-extraction.md`（本文件）

**修改**：
- `backend/cmd/server/main.go` — 删除 1200 行 seed 代码
- `backend/internal/api/formula_handler.go` — 视情况：可能需要让 Import 接受 `state` 字段（避免必须 PATCH 第二次）— **第一阶段先不动 API，多一次 PATCH 完全可接受**
- `docker-compose.yml` — 加 seed-runner 服务
- `docs/backlog.md` — 移到「已完成」
- `README.md` — 文档更新（database config 章节附近加 seed 说明）

**删除**：
- `nVar` / `nConst` / `nOp` / `nFn` / `nTableAgg` / `eEdge` helper（在 main.go 内）

## TODO

### 阶段 1

- [x] 1.1 确认设计（本文件 + 用户拍板）
- [x] 1.2 创建 `backend/seed/runner/` 目录 + 基础 Go 文件（login / list / import / publish / 占位符替换）
- [x] 1.3 单元测试占位符替换函数
- [x] 1.4 启动现 backend → 用 export API dump 31 个公式 → 落到 `backend/seed/formulas/`
- [x] 1.5 dump 2 个 lookup table → 落到 `backend/seed/tables/`
- [x] 1.6 手动编辑 bundle 文件：把 graph 里的真 UUID 替换成 `{{formula:NAME}}` / `{{table:NAME}}` 占位符
- [x] 1.7 重命名 bundle 文件为依赖序前缀（`010-` … `310-` …）
- [x] 1.8 端到端验证（postgres 容器）：wipe → seed-runner → 31 个公式 + 2 个表全部创建 + LDF/年金/責任準備金 可计算
- [x] 1.8a 删除 bundle 中的 orphan Variable 节点（验证器拒绝 disconnected nodes，但 engine 不需要它们）
- [x] 1.9 删除 main.go 中的 seed 公式/表代码（保留 admin/category）
- [x] 1.10 用 `backend/seed/embed.go` + `go:embed` 替代 `seedFormulaNames` / `seedTableNames`
- [x] 1.11 调整 `makeSeedResetHandler` 为只清理（重命名 `seed()` 为 `bootstrap()` 避免与 `seed` 包冲突）
- [x] 1.12 build & 测试（go vet / go test ./... -race / docker rebuild + 端到端 reset → 重新 seed → LDF=1.27033 验证）
- [x] 1.13 写 `backend/seed/README.md`
- [x] 1.14 提交阶段 1 commit（含 3 轮 codex review：round 1 P1 phantom + 2 P2 + 1 P3 → round 2 1 P2 + 1 P3 → round 3 LGTM）

### 阶段 2

- [ ] 2.1 Runner 加 `--dry-run` flag
- [ ] 2.2 Runner 加 `--only NAME` flag
- [ ] 2.3 Bundle 文件按 domain 分子目录
- [ ] 2.4 `Dockerfile.seed` + `docker-compose.yml` 集成
- [ ] 2.5 README 更新（项目级 README 加 seed-runner 用法）
- [ ] 2.6 提交阶段 2 commit（含 codex review）

## 完成标准

### 阶段 1
- [ ] main.go < 500 行（移除约 1200 行 seed 代码）
- [ ] `backend/seed/formulas/*.json` 24 个文件，`backend/seed/tables/*.json` 2 个文件
- [ ] `seed-runner` 在 fresh DB 上能 seed 全部 24 个公式 + 2 个表
- [ ] 已存在的公式被正确跳过（幂等）
- [ ] 跨引用占位符正确解析
- [ ] 全套 backend test 通过（`go test ./... -race`）
- [ ] 端到端：fresh DB → run server → run seed-runner → 调用 `日本損害保険 チェインラダー LDF` 计算返回正确结果（agg_ldf ≈ 1.27033）
- [ ] codex review LGTM

### 阶段 2
- [ ] Docker compose `--profile seed` 一条命令完成 seed
- [ ] dry-run 在没有 backend 启动时也能验证 bundle 合法性（解析 + 占位符引用闭合）
- [ ] codex review LGTM

## 中断记录

（如果中断了，记录当前状态、下一步是什么）
