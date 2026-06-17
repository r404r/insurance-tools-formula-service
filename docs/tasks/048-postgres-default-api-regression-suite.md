# Task #048: PostgreSQL 默认数据库 + 可复用 API 回归测试套件

## Status: done

## 需求

GHO-64（父 issue GHO-62）：将后端容器化交付路径补齐，使数据库类型可通过环境配置文件指定，
并以 **PostgreSQL 为默认数据库**；针对核心 API 行为建立一套**可复用、可重复执行**的回归测试套件，
每次执行后把结果**汇总成 markdown 落盘**便于复查。

现状盘点（已存在，不需重做）：
- `docker-compose.yml` 已支持 sqlite/postgres/mysql 三个 profile（postgres 带 sidecar）。
- `backend/internal/config` 已支持 `DB_DRIVER` / `DB_DSN` 环境变量切换驱动。
- Task #044 做过一次性的 PostgreSQL batch 计算回归，但**不是可复用的 API 套件**，也没有自动落盘机制。

缺口（本任务交付）：
1. 默认数据库切到 PostgreSQL（compose profile 默认 + `.env.example` + 文档一致）。
2. 一个黑盒 API 回归 runner，覆盖核心 API 行为（健康检查、认证、parse、formula 生命周期、
   calculate/batch-test、tables、categories、templates、权限边界），自带测试数据、可重复执行。
3. 每次运行把汇总结果写成 markdown 落盘到 `tests/reports/`。
4. 一个 `/healthz` 健康检查端点（容器就绪探针 + 回归套件就绪探针）。

## 设计

### 1. `/healthz` 健康检查端点
- `store.Store` 接口新增 `Ping(ctx) error`，三个实现各加一行 `db.PingContext`。
- 新 `internal/api/health_handler.go`：`GET /healthz` 返回 `{"status":"ok","database":"<driver>"}`，
  DB ping 失败返回 503。无需鉴权、无速率限制。
- `router.go` / `main.go` 接线。

### 2. 默认 PostgreSQL
- `.env.example`：`COMPOSE_PROFILES=postgres`，DB 段以 postgres 为激活默认（sqlite 作为可选备注）。
- `docker-compose.yml` 顶部注释、README、CLAUDE.md 默认数据库措辞统一为 postgres。
- 代码级 fallback 仍保留 sqlite（零依赖本地 `go run` / 单元测试不需要起 DB 服务器）。

### 3. 可复用 API 回归 runner
- `backend/cmd/api_regression/main.go`：纯 HTTP 黑盒客户端（cookie jar 自动带 auth_token）。
  - 配置：`BASE_URL`(默认 http://localhost:8080)、`ADMIN_USER`/`ADMIN_PASS`、`REPORT_DIR`。
  - 自建测试数据：parse → 建 formula → 建 version → 发布 → calculate → batch-test → 清理。
  - 每个 check 记录 名称/通过/耗时/详情；任一失败 exit 1。
  - 落盘 markdown：`tests/reports/api-regression-<UTC>.md` + 覆盖 `api-regression-latest.md`。
- `tests/api-regression/run.sh`：默认拉起 postgres profile（docker compose）→ 等待 /healthz →
  seed → 跑 runner；`BASE_URL` 已就绪时跳过 docker 直接打已有后端。

## 涉及文件

- `backend/internal/store/repository.go`、`backend/internal/store/{sqlite,postgres,mysql}/store.go`
- `backend/internal/api/health_handler.go`、`backend/internal/api/router.go`、`backend/cmd/server/main.go`
- `backend/cmd/api_regression/main.go`（新建）
- `tests/api-regression/run.sh`（新建）
- `.env.example`、`docker-compose.yml`、`README.md`、`CLAUDE.md`

## TODO

- [x] /healthz 端点 + Store.Ping
- [x] 默认 postgres（compose + env + 文档）
- [x] api_regression runner
- [x] run.sh 包装脚本
- [x] 本地 sqlite 后端验证 runner（go build/vet/test 全绿，报告落盘）— 21/21 PASS
- [x] postgres 路径验证（postgres:16 容器 + native backend）— 21/21 PASS
- [x] 生成首份报告到 tests/reports/（api-regression-latest.md，db=postgres）

## 完成标准

- [x] `go build ./...` / `go vet ./...` / `go test ./...` 通过
- [x] runner 对运行中的后端 100% 通过并落盘 markdown（sqlite + postgres 均 21/21）
- [x] 默认 DB = PostgreSQL，文档一致（.env.example / docker-compose.yml / README / CLAUDE.md）
