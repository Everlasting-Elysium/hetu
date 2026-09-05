# 技术选型

## 核心语言选型

| 维度 | Go | Node.js/TypeScript | Rust | .NET | Python |
|------|----|--------------------|------|------|--------|
| 开发者熟悉度 | 主力语言 | 一般 | 学习成本高 | 一般 | 熟悉但不适合服务端主力 |
| 单二进制部署 | 是，静态编译 | 否，需 Node 运行时 | 是 | 否，需 .NET 运行时 | 否，需 Python 运行时 |
| Windows Server 部署 | 交叉编译，无依赖 | 需安装 Node | 可交叉编译 | 需安装 .NET | 需安装 Python |
| Docker 镜像体积 | 极小（scratch/distroless） | 较大 | 极小 | 中等 | 较大 |
| I/O 并发（NAS 场景） | goroutine，优秀 | 事件循环，良好 | async/await，优秀 | async，良好 | GIL 限制，较差 |
| 存储生态（rclone/AList/juicefs/minio） | Go 原生，可直接嵌入 | 需 FFI 或 sidecar | 需 FFI | 需 FFI | 需 sidecar |
| CGO 依赖 | 可选，默认纯 Go | 不适用 | 不适用 | 不适用 | 不适用 |

**决策**：Go。模块路径 `github.com/Everlasting-Elysium/hetu`。

核心优势：开发者主力语言；单静态二进制使 Windows Server 和最小化 Docker 镜像部署极为简单；goroutine 模型天然适合 NAS 的高并发 I/O；存储生态（rclone、AList、juicefs、minio）均为 Go 实现，可直接嵌入或作为 sidecar 运行。

---

## 各层库选型

### HTTP 与 CLI

| 用途 | 库 | 理由 |
|------|----|------|
| HTTP 路由 | `go-chi/chi` | 轻量、符合标准库 `net/http` 接口、中间件生态完善 |
| CLI 框架 | `spf13/cobra` | Go 生态事实标准，子命令结构清晰 |
| 日志 | `log/slog`（标准库） | Go 1.21+ 内置结构化日志，无外部依赖 |
| 配置 | `caarlos0/env` | 纯环境变量驱动，适合容器化部署，无配置文件解析复杂度 |
| ID 生成 | `google/uuid`（v7） | UUID v7 单调递增，适合数据库主键排序 |

### 数据库层

| 用途 | 库 | 理由 |
|------|----|------|
| SQLite 驱动 | `modernc.org/sqlite` | 纯 Go 实现，无 CGO，见下节详述 |
| 查询代码生成 | `sqlc` | 从 SQL 生成类型安全的 Go 代码，避免手写 ORM |
| 全文检索（Phase 1） | SQLite FTS5 | 内置于 SQLite，无额外依赖 |
| 向量搜索（Phase 1） | `sqlite-vec` | 见下节开放问题 |

### 存储层

| 用途 | 库/工具 | 说明 |
|------|---------|------|
| 存储抽象 | `rclone`（Go 库或 sidecar） | 支持 70+ 后端，详见 [storage.md](./storage.md) |
| 中国网盘驱动 | AList/OpenList | 补充 rclone 不支持的网盘，详见 [storage.md](./storage.md) |

### AI 与 3D

| 用途 | 方案 | 说明 |
|------|------|------|
| AI 推理 | Python sidecar | 独立进程/容器，HTTP/gRPC 通信，详见 [ai-and-3d.md](./ai-and-3d.md) |
| 3D 缩略图 | Blender headless（`blender -b`） | sidecar，详见 [ai-and-3d.md](./ai-and-3d.md) |
| Web 3D 预览 | three.js / `<model-viewer>` | 前端，详见 [ai-and-3d.md](./ai-and-3d.md) |

---

## 为什么选 modernc.org/sqlite

`modernc.org/sqlite` 是将 SQLite C 源码通过 `cgo-free` 转译工具转为纯 Go 的实现。选择它的原因：

1. **无 CGO**：交叉编译到 Windows（`GOOS=windows GOARCH=amd64`）和 Linux（Docker）无需配置 C 工具链，`go build` 直接产出静态二进制。
2. **部署简单**：Docker 镜像可基于 `scratch` 或 `distroless`，无需安装 `libsqlite3`。
3. **功能完整**：支持 WAL 模式、FTS5、JSON 扩展等 SQLite 标准特性。
4. **接口兼容**：实现标准 `database/sql` 接口，sqlc 生成的代码无需修改即可使用。

数据库访问通过 `internal/store` 中的 `Store` 接口隔离，若未来需要迁移到 Postgres + pgvector，只需新增一个实现，上层代码不受影响。

---

## sqlite-vec 的开放问题

**sqlite-vec** 是 SQLite 的向量搜索扩展，用于 Phase 1 的 CLIP 语义搜索和视觉相似度功能。

**问题**：sqlite-vec 是 C 扩展（`.so`/`.dll`），加载它需要 CGO。这与选择 `modernc.org/sqlite`（无 CGO）的目标存在冲突。

**当前状态**：这是一个尚未最终决策的开放问题，Phase 1 实现时需要在以下方案中选择：

| 方案 | 描述 | 代价 |
|------|------|------|
| 切换到 `mattn/go-sqlite3` + sqlite-vec | 启用 CGO，加载 C 扩展 | 失去无 CGO 优势，交叉编译需配置 C 工具链 |
| 保留 `modernc.org/sqlite`，向量搜索在 Python sidecar 中实现 | AI sidecar 负责向量索引和检索（如 faiss、hnswlib） | 向量搜索与 AI sidecar 耦合，增加 sidecar 职责 |
| 等待 `modernc.org/sqlite` 支持扩展加载 | 社区方向不确定 | 时间不可控 |
| 使用纯 Go 向量库（如 `weaviate/weaviate` 嵌入模式） | 无 CGO，但引入较重依赖 | 依赖体积大，功能可能过剩 |

### 决策结论

**选定方案：保留 `modernc.org/sqlite`，向量存储为普通 SQLite BLOB，相似度计算在 Go 层完成（暴力余弦相似度）。**

| 维度 | 评估 |
|------|------|
| CGO | 不需要，保留 `modernc.org/sqlite` 无 CGO 优势 |
| 部署复杂度 | 无额外依赖，单二进制不变 |
| 性能 | 个人 NAS 规模（< 100K 资产），512 维暴力余弦搜索 < 50ms，足够 |
| 数据一致性 | 嵌入与资产元数据同库，备份/迁移一体 |
| 可演进性 | 若规模增长，可切换为纯 Go ANN（HNSW）或引入外部向量库，Store 接口隔离上层 |

排除理由：
- **(a) `mattn/go-sqlite3` + CGO**：失去交叉编译和最小镜像优势，对个人 NAS 规模过度
- **(c) Python sidecar 向量索引**：检索依赖 sidecar 存活，增加运维复杂度；嵌入数据与 SQLite 分离
- **(d) 等待 `modernc` 扩展支持**：时间不可控

实现细节：`embeddings` 表存 `float32` 数组的 BLOB（`internal/vecmath` 包负责序列化），搜索时加载候选向量在 Go 层计算余弦相似度并排序。CLIP 输出已 L2 归一化，余弦相似度退化为点积。
