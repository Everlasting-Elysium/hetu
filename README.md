# hetu(河图)

自托管、AI 原生的 **NAS + 资源管理(DAM)** 平台。采用**微内核 + 插件**架构:内核是平台,**DAM 和 NAS 是两个能力插件**,通过 `HETU_PLUGINS` 决定“插哪个就是哪个”。

设计文档见 [`docs/`](docs/)(中文,从 [docs/README.md](docs/README.md) 进入)。

## 快速开始

    # 构建
    go build -o bin/hetu ./cmd/hetu

    # 索引一个素材目录(索引不搬运原文件)
    HETU_LIBRARY_DIR=/path/to/assets ./bin/hetu scan

    # 启动服务
    HETU_LIBRARY_DIR=/path/to/assets ./bin/hetu serve
    # GET http://localhost:8080/healthz
    # GET http://localhost:8080/api/dam/assets
    # GET http://localhost:8080/api/nas/browse?path=

## 配置

全部通过环境变量,定义见 [internal/config/config.go](internal/config/config.go)。关键项:`HETU_PLUGINS`(默认 `dam,nas`)、`HETU_LIBRARY_DIR`、`HETU_DATA_DIR`、`HETU_DB_PATH`、`HETU_ADDR`。

## 技术栈

Go 内核(chi / slog / cobra)· SQLite(`modernc.org/sqlite`,纯 Go 无 CGO)+ sqlc · Python AI sidecar(Phase 1)· Blender headless 3D 缩略(Phase 1)· rclone/AList 把网盘当 S3(Phase 1)。详见 [docs/tech-stack.md](docs/tech-stack.md)。

## 当前状态(Phase 0 骨架)

存储抽象(本地)、索引不搬运、图片元数据 + 缩略、DAM/NAS 插件、`扫描 → 缩略 → 入库 → HTTP 查询` 链路已跑通并有测试覆盖。路线图见 [docs/roadmap.md](docs/roadmap.md)。
