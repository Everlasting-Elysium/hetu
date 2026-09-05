# 存储层

## 设计原则

hetu 的存储层遵循**索引不搬运**原则：文件保留在原始位置，hetu 只记录路径和元数据，不复制或移动文件。这与 Billfish 的原地索引策略一致。

所有存储访问通过 `StorageProvider` 契约接口进行，上层代码（DAM 插件、NAS 插件、Indexer）不直接操作文件系统或网盘 SDK。

---

## StorageProvider 契约

接口定义在 `internal/kernel/` 中，完整签名以代码文件为准。概念说明：

| 方法 | 说明 |
|------|------|
| `Name() string` | 返回 provider 的唯一标识符（如 `local`、`s3`） |
| `List(ctx, prefix string) ([]Entry, error)` | 列出指定前缀下的所有条目（文件和目录） |
| `Open(ctx, path string) (io.ReadCloser, error)` | 打开文件，返回可读流 |
| `Stat(ctx, path string) (FileInfo, error)` | 获取文件元信息（大小、修改时间等） |

`Entry` 和 `FileInfo` 是 `internal/domain` 中定义的值类型。

`StorageRegistry`（也在 `internal/kernel/` 中）负责注册和查找 provider：插件或 Indexer 通过 provider 名称从 registry 中取得对应实现。

---

## v0：仅本地文件系统

v0 唯一实现的 provider 是 `internal/storage/local`，对应本地文件系统。

- provider 名称：`local`
- 根路径通过配置项指定，配置项定义见 `internal/config/config.go`
- `List` 递归遍历目录，`Open` 直接打开文件，`Stat` 调用 `os.Stat`

---

## rclone：统一存储抽象层（已实现）

rclone 是 Go 实现的存储抽象工具，支持 70+ 后端（本地、S3、Google Drive、OneDrive、Dropbox、阿里云 OSS、腾讯 COS 等）。

### 集成方式

hetu 通过 rclone RC daemon（`rclone rcd --rc-serve`）的 HTTP API 集成：

- **List / Stat**：调用 RC JSON API（`POST /operations/list`、`POST /operations/stat`）
- **Open**：通过 `--rc-serve` 暴露的 HTTP 文件服务器读取文件内容，支持 Range 请求（`io.ReadSeekCloser`）

实现代码在 `internal/storage/rclone/`，provider 名称为 `rclone`。

### 配置项

所有配置通过环境变量，定义见 `internal/config/config.go`：

| 环境变量 | 说明 | 默认值 |
|----------|------|--------|
| `HETU_RCLONE_ADDR` | rclone RC daemon 地址（如 `localhost:5572`）。为空则不注册 rclone provider | 空（禁用） |
| `HETU_RCLONE_REMOTE` | rclone remote 名称 | `remote:` |
| `HETU_RCLONE_USER` | Basic Auth 用户名（可选） | 空 |
| `HETU_RCLONE_PASS` | Basic Auth 密码（可选） | 空 |
| `HETU_NAS_PROVIDER` | NAS 插件浏览时使用的 provider（`local` 或 `rclone`） | `local` |

`HETU_NAS_PROVIDER` 必须指向一个已注册的 provider：设为 `rclone` 时必须同时设置 `HETU_RCLONE_ADDR`，否则启动时 fail-fast 报错。

### 部署

Docker Compose 中已预配 rclone 服务（profile `storage`）：

```bash
# 1. 配置 remote
#    编辑 deploy/rclone/rclone.conf（或用 rclone config 生成）
# 2. 启动
docker compose --profile storage up
# 3. hetu 侧设置：启用 rclone provider 并让 NAS 插件使用它
HETU_RCLONE_ADDR=rclone:5572 HETU_RCLONE_REMOTE=remote: HETU_NAS_PROVIDER=rclone ./bin/hetu serve
```

rclone daemon 以 `rcd --rc-addr :5572 --rc-serve --rc-no-auth` 启动，RC API 和文件服务共用同一端口。生产环境应配置 `--rc-user`/`--rc-pass` 并在 hetu 侧设置 `HETU_RCLONE_USER`/`HETU_RCLONE_PASS`。

---

## AList / OpenList：中国网盘驱动（路线图）

部分中国网盘（如百度网盘、夸克网盘、115 等）rclone 不支持或支持不稳定。AList/OpenList 专门针对这些网盘提供驱动，并暴露 WebDAV 或 S3 兼容接口。

集成策略：AList/OpenList 作为 sidecar 运行，hetu 通过其 S3/WebDAV 接口访问，与 rclone sidecar 的集成方式一致，不需要单独的 provider 实现。

---

## 存储提供者路线图

| 阶段 | Provider | 状态 |
|------|----------|------|
| Phase 0 | 本地文件系统（`internal/storage/local`） | 已实现 |
| Phase 0 后 | rclone（RC daemon sidecar，`internal/storage/rclone`） | **已实现** |
| Phase 0 后 | AList/OpenList sidecar | 路线图 |

新增 provider 只需实现 `StorageProvider` 接口并注册到 `StorageRegistry`，不影响任何上层代码。
