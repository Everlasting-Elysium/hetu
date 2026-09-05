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

## rclone：统一存储抽象层（路线图）

rclone 是 Go 实现的存储抽象工具，支持 70+ 后端（本地、S3、Google Drive、OneDrive、Dropbox、阿里云 OSS、腾讯 COS 等）。

hetu 计划以两种方式之一集成 rclone：

| 集成方式 | 描述 | 适用场景 |
|----------|------|----------|
| Go 库嵌入 | 将 rclone 作为 Go 库直接引入，在进程内调用 | 需要低延迟、紧密集成 |
| sidecar 进程 | 运行 `rclone serve s3`，将任意后端暴露为 S3 API | 部署简单，隔离性好 |

`rclone serve s3` 的关键能力：将 70+ 后端（包括大量中国网盘）统一暴露为 S3 兼容 API，hetu 只需实现一个 S3 StorageProvider，即可访问所有 rclone 支持的后端。

**v0 不实现 rclone 集成**，rclone provider 是 Phase 0 完成后的下一个 provider 实现目标。

---

## AList / OpenList：中国网盘驱动（路线图）

部分中国网盘（如百度网盘、夸克网盘、115 等）rclone 不支持或支持不稳定。AList/OpenList 专门针对这些网盘提供驱动，并暴露 WebDAV 或 S3 兼容接口。

集成策略：AList/OpenList 作为 sidecar 运行，hetu 通过其 S3/WebDAV 接口访问，与 rclone sidecar 的集成方式一致，不需要单独的 provider 实现。

---

## 存储提供者路线图

| 阶段 | Provider | 状态 |
|------|----------|------|
| Phase 0 | 本地文件系统（`internal/storage/local`） | 已实现 |
| Phase 0 后 | rclone（Go 库或 sidecar） | 路线图 |
| Phase 0 后 | AList/OpenList sidecar | 路线图 |

新增 provider 只需实现 `StorageProvider` 接口并注册到 `StorageRegistry`，不影响任何上层代码。
