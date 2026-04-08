# Task #015: MySQL Store 实现

## Status: in-progress

## 需求

在已有 SQLite / PostgreSQL Store 的基础上，追加 MySQL Store 实现，
使项目能通过 `DB_DRIVER=mysql` 切换到 MySQL 后端数据库。

## 设计

### 后端

- 新增 `backend/internal/store/mysql/` 包，实现与 PostgreSQL 相同的 `store.Store` 接口
- 主要与 PostgreSQL 的差异：
  - 占位符：`$N` → `?`（MySQL 使用 `?`，与 SQLite 相同）
  - 时间戳：MySQL 用 `DATETIME(6)`，存储 `time.RFC3339Nano` 字符串并手动解析（与 SQLite 方式相同）
  - 大小写不敏感搜索：`ILIKE` → `LIKE`（MySQL `LIKE` 默认大小写不敏感）
  - Upsert：`INSERT ... ON DUPLICATE KEY UPDATE`（代替 PostgreSQL 的 `ON CONFLICT`）
  - 并发 publish 锁：`SELECT ... FOR UPDATE`（MySQL InnoDB 同样支持）
  - FK 错误检测：`*mysql.MySQLError` code `1451` / `1452`
  - 连接池：`MaxOpenConns=25`
- 驱动：`github.com/go-sql-driver/mysql`
- `backend/cmd/server/main.go`：`case "mysql"` 调用 `mysql.New(dsn)`

### 涉及文件

- `backend/internal/store/mysql/store.go`（新建）
- `backend/internal/store/mysql/settings.go`（新建）
- `backend/cmd/server/main.go`（修改：追加 mysql case）
- `backend/go.mod` + `backend/go.sum`

## TODO

- [x] 创建任务文件
- [x] `go get github.com/go-sql-driver/mysql`
- [x] `backend/internal/store/mysql/store.go`
- [x] `backend/internal/store/mysql/settings.go`
- [x] `backend/cmd/server/main.go` 追加 mysql case
- [x] `go build` 通过
- [x] codex review + fix P1/P2
- [x] 提交

## 完成标准

- [x] `go build` 通过
- [x] codex review 无 P1/P2
- [x] commit
