# Insurance Formula Calculation Engine - Requirements Document

## 1. Project Overview

构建一个面向保险行业的公式计算引擎服务，涵盖人寿保险、财产保险和车险三大领域，提供可视化公式编辑、版本管理、高精度计算和高性能API。

## 2. Functional Requirements

### 2.1 Formula Visual Editor (公式可视化编辑器)

- **FR-001**: 提供基于 react-flow 的可视化DAG编辑画布，支持拖拽创建/连接节点
- **FR-002**: 支持8种节点类型：变量(Variable)、常量(Constant)、运算符(Operator)、函数(Function)、子公式引用(SubFormula)、查表(TableLookup)、条件分支(Conditional)、聚合(Aggregate)
- **FR-003**: 支持双模式编辑 — 可视化拖拽模式和文本表达式模式，两种模式可双向转换
- **FR-004**: 编辑器实时DAG验证（环检测、类型检查、连接验证）
- **FR-005**: 编辑器内置测试面板，可输入样本数据即时计算查看结果
- **FR-006**: 支持 dagre 自动布局

### 2.2 Version Management (版本管理)

- **FR-010**: 公式版本状态机：Draft(草稿) -> Published(已发布) -> Archived(已归档)
- **FR-011**: 每个公式同一时间只能有一个 Published 版本
- **FR-012**: 版本全量快照存储，支持版本间 Diff 比较
- **FR-013**: 版本回滚功能（复制历史版本创建新 Draft）
- **FR-014**: 版本时间线可视化展示

### 2.3 Calculation API (计算API)

- **FR-020**: 提供单次计算 API (POST /api/v1/calculate)
- **FR-021**: 提供批量计算 API (POST /api/v1/calculate/batch)
- **FR-022**: 提供公式验证 API (POST /api/v1/calculate/validate)
- **FR-023**: 计算结果以字符串返回，避免精度丢失
- **FR-024**: 返回中间计算结果用于调试

### 2.4 Insurance Domains (保险领域)

- **FR-030**: 人寿保险：生命表查询、保费计算、准备金计算、年金因子
- **FR-031**: 财产保险：风险评分、费率计算、赔付率计算
- **FR-032**: 车险：车辆分类、驾驶员风险因子、无赔款优惠(NCD)、保障计算
- **FR-033**: 各领域提供预置公式模板，用户可克隆和自定义

### 2.5 RBAC (权限管理)

- **FR-040**: 4种角色：Admin(管理员)、Editor(编辑者)、Reviewer(审核者)、Viewer(只读)
- **FR-041**: JWT 令牌认证
- **FR-042**: Editor 可创建/编辑公式，Reviewer 可审核发布，Admin 管理用户
- **FR-043**: 首个注册用户自动获得 Admin 角色

### 2.6 Internationalization (国际化)

- **FR-050**: 支持中文(zh)、日文(ja)、英文(en) 三语切换
- **FR-051**: 前端使用 i18next 框架，翻译文件独立管理

## 3. Non-Functional Requirements

### 3.1 High Precision (高精度)

- **NFR-001**: 支持小数点后18位精度输出
- **NFR-002**: 中间计算精度支持至28位
- **NFR-003**: 使用 shopspring/decimal 库实现金融级定点运算

### 3.2 High Performance (高性能)

- **NFR-010**: 复杂公式自动DAG拓扑排序，识别可并行的子公式
- **NFR-011**: 同层级节点通过 Go goroutine + errgroup 并行执行
- **NFR-012**: 小公式(<8节点)跳过并行化以避免 goroutine 开销
- **NFR-013**: LRU 缓存避免重复计算

### 3.3 Database Flexibility (数据库灵活性)

- **NFR-020**: 同时支持嵌入式 SQLite（开发/轻量部署）和 PostgreSQL/MySQL（生产环境）
- **NFR-021**: 通过 Repository 接口模式实现数据库无关性
- **NFR-022**: 使用 golang-migrate 管理数据库迁移

## 4. Technology Stack

| Layer | Technology |
|-------|-----------|
| Frontend | React + TypeScript + Vite |
| UI Styling | Tailwind CSS |
| Visual Editor | react-flow v12+ |
| State Management | Zustand + TanStack Query |
| i18n | i18next + react-i18next |
| Backend | Go + chi router |
| Precision | shopspring/decimal |
| Embedded DB | modernc.org/sqlite (pure Go) |
| Production DB | PostgreSQL (pgx/v5) / MySQL |
| Auth | JWT (golang-jwt/jwt/v5) + bcrypt |
| Logging | rs/zerolog |
