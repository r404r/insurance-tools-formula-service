# Task #014: PostgreSQL Store 实现

## Status: done

## 需求

项目目前只有 SQLite 存储实现。需要追加 PostgreSQL 存储，使生产环境能使用 PostgreSQL 作为后端数据库。
通过 `DB_DRIVER=postgres` 环境变量切换，`DB_DSN` 为 PostgreSQL 连接字符串。

## 设计

### 后端

- 新增 `backend/internal/store/postgres/` 包，实现与 SQLite 完全相同的 `store.Store` 接口
- 主要差异：
  - 占位符：`?` → `$1, $2, ...`
  - 时间戳列类型：`TEXT` → `TIMESTAMPTZ`，可直接扫描为 `time.Time`
  - 大小写不敏感搜索：`LIKE` → `ILIKE`
  - FK 违约错误检测：通过 `pq.Error.Code == "23503"` 而非字符串匹配
  - 连接池：PostgreSQL 支持更大并发，默认 `MaxOpenConns=25`
- 驱动：`github.com/lib/pq`（标准 `database/sql` 兼容）
- `backend/cmd/server/main.go`：根据 `cfg.Database.Driver` 选择存储实现

### 涉及文件

- `backend/internal/store/postgres/store.go`（新建）
- `backend/internal/store/postgres/settings.go`（新建）
- `backend/cmd/server/main.go`（修改：追加 driver dispatch）
- `backend/go.mod` + `backend/go.sum`（追加 `github.com/lib/pq`）

## TODO

- [x] 创建任务文件
- [x] `go get github.com/lib/pq`
- [x] `backend/internal/store/postgres/store.go`
- [x] `backend/internal/store/postgres/settings.go`
- [x] `backend/cmd/server/main.go` 追加 driver dispatch
- [x] `go build` 通过
- [x] codex review + fix P1/P2
- [x] 提交

## 完成标准

- [x] `DB_DRIVER=postgres go build` 通过（无需实际 PostgreSQL 连接）
- [x] codex review 无 P1/P2
- [x] commit
