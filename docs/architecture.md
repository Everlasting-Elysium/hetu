# 架构设计

## 微内核 + 插件模型

内核（`internal/kernel`）是一个服务注册中心，不包含任何业务逻辑。它向插件暴露以下服务：

- `Store`：数据库访问接口
- `StorageRegistry`：存储提供者注册与查找
- `AssetRegistry`：资产处理器注册与查找
- `EventBus`：异步事件发布/订阅
- `JobQueue`：后台任务队列

插件通过 `Plugin` 契约接入内核。插件在 `Init(ctx, *Kernel)` 中获得 `*Kernel` 引用，从而访问上述所有服务，无需直接依赖其他插件。

**关键约束**：DAM 插件和 NAS 插件之间不得直接调用，只能通过内核服务（EventBus、Store）间接协作。

---

## 插件机制四种方案对比

| 方案 | 描述 | 隔离性 | 部署复杂度 | v0 采用 |
|------|------|--------|------------|---------|
| 配置期编译模块 | 插件编译进同一二进制，通过环境变量 `HETU_PLUGINS` 启用 | 无进程隔离 | 最低，单二进制 | 是 |
| subprocess + gRPC | hashicorp/go-plugin 风格，插件为独立子进程，通过 gRPC 通信 | 进程隔离 | 中等 | 路线图 |
| 独立容器 | 每个插件为独立容器，通过 docker compose profiles 编排 | 完全隔离 | 较高，需 Docker | 路线图 |
| WASM（extism） | 插件编译为 WASM 模块，运行在沙箱中 | 沙箱隔离 | 低（无需 Docker） | 路线图 |

**v0 决策**：配置期编译模块。插件列表由环境变量 `HETU_PLUGINS`（逗号分隔，如 `dam,nas`）控制，配置项定义见 `internal/config/config.go`。所有插件代码编译进同一二进制，启动时按配置决定是否调用 `Init`。

---

## 数据流

### 索引流（扫描入库）

```
CLI scan 命令
  → internal/index.Indexer
    → StorageProvider.List（遍历文件）
    → AssetRegistry.Match（按扩展名选 AssetHandler）
    → AssetHandler.Extract（提取元数据）
    → AssetHandler.Thumbnail（生成缩略图）
    → Store.UpsertAsset（写入数据库）
```

### HTTP 请求流

```
HTTP 请求
  → internal/api（chi router）
    → 插件挂载的路由（DAM / NAS）
    → 插件业务逻辑
    → Store / StorageProvider / AssetRegistry
    → JSON 响应
```

### AI 打标流（Phase 1）

详见 [ai-and-3d.md](./ai-and-3d.md)。

---

## Go 目录结构

```
github.com/Everlasting-Elysium/hetu
├── cmd/hetu/main.go              # 入口，<=50 LOC，仅做 cobra 组装
├── internal/
│   ├── cli/                      # cobra 子命令：root / serve / scan
│   ├── config/                   # 环境变量配置，使用 caarlos0/env 解析
│   ├── obs/                      # 可观测性：log/slog 初始化与封装
│   ├── domain/                   # 值类型与哨兵错误
│   │   └── ...                   # OwnerID, AssetID, AssetKind, Asset, Meta, Entry, FileInfo
│   ├── kernel/                   # 微内核：Kernel struct + 所有契约接口
│   │   └── ...                   # Plugin, Route, StorageProvider, StorageRegistry,
│   │                             # AssetHandler, AssetRegistry, Store, EventBus, JobQueue
│   ├── storage/
│   │   └── local/                # 本地文件系统 StorageProvider（v0 唯一实现）
│   ├── asset/
│   │   └── image/                # 图片 AssetHandler：元数据提取 + 纯 Go 缩略图
│   ├── index/                    # Indexer：扫描 → 处理 → 入库的编排逻辑
│   ├── store/                    # 数据库层
│   │   ├── schema.sql            # 建表 DDL
│   │   ├── queries/              # sqlc 查询文件（.sql）
│   │   ├── db/                   # sqlc 生成代码（不手动编辑）
│   │   └── sqlite.go             # Store 接口的 SQLite 实现
│   ├── plugins/
│   │   ├── dam/                  # DAM 插件：路由 + 业务逻辑
│   │   └── nas/                  # NAS 插件：路由 + 业务逻辑
│   └── api/                      # chi server，挂载已启用插件的路由
├── ai/                           # Python AI sidecar（独立进程/容器）
└── deploy/                       # Dockerfile + docker-compose
    └── docker-compose.yml        # services: core, rclone, blender, ai
```

每个目录的职责说明：

| 路径 | 职责 |
|------|------|
| `cmd/hetu/main.go` | 程序入口，仅组装 cobra，不含业务逻辑 |
| `internal/cli` | 命令行子命令定义（serve 启动 HTTP，scan 触发索引） |
| `internal/config` | 从环境变量读取配置，结构体定义是配置项的唯一来源 |
| `internal/obs` | slog 初始化，统一日志格式 |
| `internal/domain` | 全局值类型和哨兵错误，无外部依赖 |
| `internal/kernel` | 微内核：Kernel struct + 所有契约接口定义 |
| `internal/storage/local` | 本地文件系统的 StorageProvider 实现 |
| `internal/asset/image` | 图片资产处理器，纯 Go 实现，无 CGO |
| `internal/index` | 扫描编排：遍历 → 匹配处理器 → 提取 → 入库 |
| `internal/store` | SQLite 数据库层，sqlc 生成代码 + Store 接口实现 |
| `internal/plugins/dam` | DAM 插件：标签、文件夹、智能集合等路由 |
| `internal/plugins/nas` | NAS 插件：文件浏览、下载、分享等路由 |
| `internal/api` | chi router 组装，挂载插件路由，处理中间件 |
| `ai/` | Python AI sidecar，提供 HTTP/gRPC 接口供内核调用 |
| `deploy/` | 容器化部署配置 |

---

## 契约接口说明

所有接口定义在 `internal/kernel/` 下，以下为概念说明，完整签名以代码文件为准。

### Plugin

```
Name() string
Init(ctx context.Context, k *Kernel) error
Routes() []Route
```

插件通过 `Init` 获得 `*Kernel`，从中取得所需服务。`Routes()` 返回该插件注册的 HTTP 路由列表，由 `internal/api` 统一挂载到 chi router。

### StorageProvider

```
Name() string
List(ctx, prefix) ([]Entry, error)
Open(ctx, path) (io.ReadCloser, error)
Stat(ctx, path) (FileInfo, error)
```

统一抽象本地文件系统、网盘、S3 等后端。详见 [storage.md](./storage.md)。

### AssetHandler

```
Match(ext string) bool
Kind() AssetKind
Extract(ctx, src StorageProvider) (Meta, error)
Thumbnail(ctx, src StorageProvider, width int) ([]byte, error)
```

每种资产类型（图片、视频、3D 等）对应一个实现。`AssetRegistry` 按文件扩展名路由到对应处理器。

### Store

```
EnsureOwner(ctx, id OwnerID) error
UpsertAsset(ctx, asset Asset) error
ListAssets(ctx, ownerID OwnerID, filter Filter) ([]Asset, error)
Close() error
```

数据库访问的唯一入口。当前实现为 `internal/store/sqlite.go`，接口设计保证后续可替换为 Postgres 实现而不影响上层代码。

### EventBus

发布/订阅接口，用于插件间解耦通信（如索引完成后通知 AI 打标任务）。接口定义见 `internal/kernel/`。

### JobQueue

后台任务队列，用于异步执行耗时操作（缩略图生成、AI 打标等）。接口定义见 `internal/kernel/`。
