# 数据模型

数据库 DDL 定义在 `internal/store/schema.sql`，sqlc 查询文件在 `internal/store/queries/`，生成代码在 `internal/store/db/`（不手动编辑）。

所有表均预留 `owner_id` 字段，引用 `users.id`。当前 v0 为单用户，`owner_id` 固定为系统默认用户；多用户能力在 Phase 2 叠加，不需要修改表结构。

---

## 表清单

### users

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID v7) | 主键 |
| name | TEXT | 显示名称 |
| created_at | DATETIME | 创建时间 |

v0 仅有一条系统用户记录。

---

### assets

核心资产表，每条记录对应一个被索引的文件。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID v7) | 主键 |
| owner_id | TEXT | 外键 → users.id |
| storage_path | TEXT | 存储提供者中的路径（相对于 provider 根） |
| kind | TEXT | 资产类型（image / video / model3d / document / other） |
| name | TEXT | 文件名（不含扩展名） |
| ext | TEXT | 扩展名（小写，含点，如 `.jpg`） |
| size | INTEGER | 文件字节数 |
| hash | TEXT | 文件内容哈希（SHA-256），用于去重检测 |
| thumb_path | TEXT | 缩略图存储路径，可为空 |
| width | INTEGER | 图片/视频宽度（像素），非图像类型为 NULL |
| height | INTEGER | 图片/视频高度（像素），非图像类型为 NULL |
| created_at | DATETIME | 文件创建时间（来自文件系统或 EXIF） |
| indexed_at | DATETIME | hetu 最后一次索引该文件的时间 |
| current_version_id | TEXT | 当前版本指针 → `asset_versions.id`；空字符串表示无显式版本（锚点行自身即唯一隐式版本），详见 [asset_versions](#asset_versions) |

**版本解析（issue #58）**：`storage_path` / `hash` 始终锚定最初被索引的原始文件（扫描、去重、relocate 均以其为准，因此版本功能不影响这些链路）。而读取接口（`GetAsset` / `ListAssets` / `ListAssetsFiltered` / `SearchAssets`）通过 `LEFT JOIN asset_versions` + `COALESCE` 将 `thumb_path` / `width` / `height` 解析为**当前版本**的值，使缩略/搜索反映当前版本，无需把版本数据写回 `assets`（否则会被下次扫描的 `UpsertAsset` 覆盖）。

---

### folders

文件夹层级结构，用于 DAM 插件的虚拟组织（与文件系统路径解耦）。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID v7) | 主键 |
| owner_id | TEXT | 外键 → users.id |
| parent_id | TEXT | 外键 → folders.id，根文件夹为 NULL |
| name | TEXT | 文件夹名称 |
| path | TEXT | 从根到当前节点的完整路径（冗余存储，加速查询） |

---

### tags

层级标签树。标签可嵌套（`parent_id` 自引用），支持颜色标记。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID v7) | 主键 |
| owner_id | TEXT | 外键 → users.id |
| parent_id | TEXT | 外键 → tags.id，顶级标签为 NULL |
| name | TEXT | 标签名称 |
| color | TEXT | 颜色标识（如 `#FF5733`），可为空 |

---

### asset_tags

资产与标签的多对多关联，记录标签来源（手动或 AI）。

| 字段 | 类型 | 说明 |
|------|------|------|
| asset_id | TEXT | 外键 → assets.id |
| tag_id | TEXT | 外键 → tags.id |
| source | TEXT | 来源：`manual`（用户手动）或 `ai`（AI 打标） |

联合主键：`(asset_id, tag_id)`。

---

### annotations

分层元数据存储（已实现）。每条记录是一个键值对，附带层标识和模型信息。

| 字段 | 类型 | 说明 |
|------|------|------|
| asset_id | TEXT | 外键 → assets.id |
| layer | TEXT | 层标识：`manual` / `ai` / `extracted`（规则见 [ai-and-3d.md](./ai-and-3d.md)） |
| key | TEXT | 元数据键（如 `caption`、`rating`、`exif.iso`）；SQLite 关键字，DDL 中以 `"key"` 引用 |
| value | TEXT | 元数据值（JSON 序列化） |
| model | TEXT | AI 模型标识，仅 `ai` 层有值；沿用 `NOT NULL DEFAULT ''` 约定，非 `ai` 层为空字符串 |
| created_at | INTEGER | 写入时间（unix 秒） |

联合主键：`(asset_id, layer, key)`。

颜色提取（[internal/asset/image](../internal/asset/image/palette.go) → [internal/index](../internal/index/palette.go)）向 `extracted` 层写入两条记录：`key=palette`（`[{hex,weight},…]` 主色在前的 JSON 数组）与 `key=dominant`（主色 `"#rrggbb"` JSON 字符串）。

---

### asset_colors

颜色搜索索引（已实现），由 `extracted` 层调色板派生。每条记录是一张图的一个色板色，预存 CIE-Lab 坐标，使查询只需对候选行计算 CIEDE2000 距离（无法在 SQL 内完成）。写入见 [internal/store/palette.go](../internal/store/palette.go)。

| 字段 | 类型 | 说明 |
|------|------|------|
| asset_id | TEXT | 外键 → assets.id |
| owner_id | TEXT | 外键 → users.id，按库主检索 |
| ord | INTEGER | 色板序号，`0` 为主色，其余按权重降序 |
| hex | TEXT | 颜色 `#rrggbb` |
| l / a / b | REAL | CIE-Lab 坐标（D65），预计算用于距离排序 |
| weight | REAL | 该色占图像像素的比例（0..1） |

联合主键：`(asset_id, ord)`；`owner_id` 上有索引。重新索引时按 `asset_id` 整体删除后重写。

检索接口：`GET /api/dam/search?color=<hex>&tol=<ΔE00>&limit=<n>`，按主色到查询色的 CIEDE2000 距离升序返回相近资产（`tol` 默认见 [search.go](../internal/plugins/dam/search.go) 的 `defaultColorTol`）。

---

### asset_versions

资产的**版本/修订历史**（[#58](https://github.com/Everlasting-Elysium/hetu/issues/58)）。同一资产的多次迭代（设计稿 v1/v2…）成组管理，可列出/切换当前/删除旧版；缩略/搜索反映当前版本。DDL 与实现见 [schema.sql](../internal/store/schema.sql)、[queries/version.sql](../internal/store/queries/version.sql)、[store/sqlite_versions.go](../internal/store/sqlite_versions.go)、[plugins/dam/versions.go](../internal/plugins/dam/versions.go)。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID v7) | 主键 |
| asset_id | TEXT | 外键 → assets.id，同一资产的所有版本共享 |
| owner_id | TEXT | 外键 → users.id |
| version_no | INTEGER | 版本序号（从 1 递增），`(asset_id, version_no)` 唯一 |
| provider | TEXT | 该版本字节所在的存储提供者 |
| storage_path | TEXT | 该版本文件路径。version 1 为原地索引的原始文件（锚点路径，在 `ManagedDirName` 之外）；version 2+ 为经 API 上传、拷贝进 `ManagedDirName` 的副本 |
| hash | TEXT | 该版本内容哈希（SHA-256） |
| size | INTEGER | 该版本字节数 |
| thumb_path | TEXT | 该版本缩略图路径（`{ThumbDir}/{versionID}.jpg`；version 1 复用锚点缩略图），供读取时 COALESCE 解析当前版本 |
| width / height | INTEGER | 该版本尺寸，供读取时 COALESCE 解析当前版本 |
| note | TEXT | 版本备注（上传时可选填；回填的 version 1 为 `initial`） |
| created_at | INTEGER | 写入时间（unix 秒） |

**设计模型（parse-don't-validate）**：
- **锚定不变**：`assets.storage_path` / `hash` 永久锚定最初索引的原始文件；「设为当前」只翻转 `assets.current_version_id`，从不改写锚点。扫描的 missing-detection、hash 自动重连、去重（[#22](https://github.com/Everlasting-Elysium/hetu/issues/22)）、relocate（[#45](https://github.com/Everlasting-Elysium/hetu/issues/45)）均以 `assets` 锚点为准，因此版本功能对它们零影响。
- **惰性 v1 回填**：首次为某资产上传新版本时，先用锚点当前状态合成 version 1（`note=initial`，`storage_path` 指向原地原始文件），再把上传文件作为 version 2 并设为当前。`current_version_id=''` 是「无显式版本」的哨兵，避免为绝大多数从不加版本的资产在扫描热路径写入冗余行。
- **受管存储**：上传版本经窄接口 `kernel.StorageWriter`（本地 provider 实现，调用处 type-assert；契约与 `PaletteExtractor` 等可选能力一致）拷贝到 `<ManagedDirName>/versions/<assetID>/<versionID>/<filename>`。`ManagedDirName`（`.hetu`）被扫描 walk 跳过、被 NAS 浏览隐藏，因此版本副本永不被当作新资产索引。
- **删除安全**：不能删除当前版本（须先切换）；删除旧版仅移除受管路径（`ManagedDirName` 下）的物理文件与该版本专属缩略图（`{versionID}.jpg`），永不触碰用户原地原始文件（version 1）与其共享缩略图。清空回收站（`PurgeTrash`）级联删除对应版本行（物理文件保留，与既有缩略图清理行为一致）。

**HTTP 接口**（`internal/plugins/dam/versions.go`、`version_upload.go`）：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/dam/assets/{id}/versions` | 列出版本（版本号降序，标记当前） |
| POST | `/api/dam/assets/{id}/versions` | 上传新版本（multipart：`file`、可选 `note`），自动设为当前 |
| POST | `/api/dam/assets/{id}/versions/{no}/current` | 切换当前版本（回滚） |
| DELETE | `/api/dam/assets/{id}/versions/{no}` | 删除旧版（当前版本返回 409） |

**已知限制 / 后续（本 issue 范围外）**：版本缩略图仅对有处理器的类型生成（当前为图片）；未提供逐版本缩略图 serve 端点（当前版本经 `GET /api/dam/assets/{id}/thumb` 反映）；purge 仅级联版本 DB 行，受管版本物理文件保留（与既有缩略图 purge 行为一致）。

---

### shares

分享链接，支持过期时间、密码保护、只读权限。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID v7) | 主键 |
| owner_id | TEXT | 外键 → users.id |
| target_type | TEXT | 分享目标类型：`asset` / `folder` / `tag` |
| target_id | TEXT | 分享目标的 ID |
| token | TEXT | URL 中的分享令牌，唯一索引 `idx_shares_token` |
| expires_at | INTEGER | 过期时间（unix 秒），NULL 表示永不过期 |
| password_hash | TEXT | 密码哈希；沿用 `NOT NULL DEFAULT ''` 约定，空字符串表示无密码 |
| permission | TEXT | 权限：`read`（只读），默认 `read` |
| created_at | INTEGER | 创建时间（unix 秒） |

分享 API（创建/校验令牌、密码、过期）属 [#4](https://github.com/Everlasting-Elysium/hetu/issues/4)，本表仅提供持久化：`CreateShare` / `GetShareByToken`。

---

### jobs

后台任务队列持久化表，用于缩略图生成、AI 打标等异步任务。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID v7) | 主键 |
| owner_id | TEXT | 外键 → users.id |
| type | TEXT | 任务类型（如 `thumbnail`、`ai_tag`、`3d_render`） |
| status | TEXT | 状态：`pending` / `running` / `done` / `failed`，默认 `pending` |
| payload | TEXT | JSON 序列化的任务参数 |
| created_at | INTEGER | 入队时间（unix 秒） |

任务的执行/消费由任务运行时负责（`kernel.JobQueue` 与 [#8](https://github.com/Everlasting-Elysium/hetu/issues/8)/[#9](https://github.com/Everlasting-Elysium/hetu/issues/9)），本表仅提供持久化：`EnqueueJob` / `UpdateJobStatus` / `ListJobs`。

---

## Phase 1 表

### assets_fts（FTS5 虚拟表，已实现）

SQLite FTS5 全文检索虚拟表，为**工作区级**全文检索提供支撑（跨文件夹按文件名/标签/描述检索）。建表语句与同步触发器定义在 `internal/store/schema.sql`。

**表结构**（非 contentless，`tokenize='unicode61'` 支持中文）：

| 列 | 说明 |
|------|------|
| name | 资产文件名，来自 `assets.name` |
| tags | 空格分隔的标签名称，由 `asset_tags` JOIN `tags` 聚合 |
| description | 最高优先级的 caption 注释值（manual > ai > extracted），来自 `annotations` 表 |

**同步机制**（8 个触发器，全部定义在 `schema.sql`）：

| 触发器 | 表 | 事件 | 作用 |
|--------|-----|------|------|
| `trg_assets_ai` | assets | INSERT | 插入 FTS 行（tags/description 为空，新资产尚无标签/注释） |
| `trg_assets_au` | assets | UPDATE | 删除旧 FTS 行，重建含当前 tags+description 的新行 |
| `trg_assets_ad` | assets | DELETE | 删除 FTS 行 |
| `trg_asset_tags_ai` | asset_tags | INSERT | 重建该资产的 FTS 行（tags 列更新） |
| `trg_asset_tags_ad` | asset_tags | DELETE | 重建该资产的 FTS 行（tags 列更新） |
| `trg_annotations_ai_caption` | annotations | INSERT (key='caption') | 重建该资产的 FTS 行（description 列更新） |
| `trg_annotations_au_caption` | annotations | UPDATE (key='caption') | 重建该资产的 FTS 行（description 列更新） |
| `trg_annotations_ad_caption` | annotations | DELETE (key='caption') | 重建该资产的 FTS 行（description 列更新） |

非 contentless 表可直接按 rowid 删除（`DELETE FROM assets_fts WHERE rowid = ?`），无需追踪原始值。`rowid` 与 `assets.rowid` 对齐。

**查询链路**：
- 解析器 `internal/search/parser.go` 把用户查询（`name:` `tag:` `desc:` 字段限定 + `AND`/`OR`/`NOT` 布尔 + 引号短语）转成参数化的 FTS5 `MATCH` 表达式，字段白名单 + 值全部加引号转义防注入；空/纯操作符查询返回 `ErrEmptyQuery`。
- 存储层 `internal/store/sqlite.go` 的 `SearchAssets` 手写 SQL（sqlc 不支持 FTS5 虚拟表），`JOIN assets` 后按 `assets_fts.rank`（bm25）相关度升序返回；非法 MATCH 表达式映射为 `domain.ErrInvalidQuery`。
- HTTP 接口 `GET /api/dam/search?q=`（`internal/plugins/dam/search.go`），空查询/非法查询返回 400，`limit` 限制在 `[1,200]`。

**升级兼容**：`assets_fts` 与触发器由 `schema.sql` 每次 `Open` 幂等重建。`PRAGMA user_version` 门控迁移与回填：`migrateFTS` 在版本低于当前时先 DROP 旧 FTS 表和触发器，再由 `schema.sql` 重建新结构；`backfillFTS` 随后将所有已有资产（含当前 tags 和 caption）写入 FTS 索引。变更 FTS 结构（如换 tokenizer）时递增 `ftsSchemaVersion`（当前为 2）并补迁移。

**已知限制 / 后续（本 issue 范围外）**：
- **CJK 分词**：`unicode61` 把连续中文当作单个 token，无法子串匹配（如「海滩」搜不到「日落海滩风景」）。待有真实中文数据后，评估切换 `trigram`（支持子串，但需 ≥3 字符、改变英文为子串匹配与排序语义）或 ICU。切换需借 `user_version` 做 FTS 重建。
- **并发**：默认 journal 模式下 `hetu serve`（读）与 `hetu scan`（写，含 FTS 触发器）并发可能 `SQLITE_BUSY`；后续考虑 WAL + `busy_timeout`。
- **多词字段值**：`name:日落 海滩` 中字段限定只作用于第一个词（`海滩` 退化为全列词），需要多词请用引号：`name:"日落 海滩"`。

### embeddings（已实现）

存储 CLIP 向量嵌入，用于语义搜索和视觉相似度。

| 字段 | 类型 | 说明 |
|------|------|------|
| asset_id | TEXT | 外键 → assets.id，主键 |
| embedding | BLOB | 向量数据（float32 小端序数组，`internal/vecmath` 负责序列化） |
| model | TEXT | 生成该嵌入的模型标识（如 `openai/clip-vit-base-patch32`） |
| created_at | INTEGER | 写入时间（unix 秒） |

**向量方案**：不使用 sqlite-vec C 扩展，改为普通 SQLite BLOB 存储 + Go 层暴力余弦相似度计算。CLIP 输出已 L2 归一化，余弦相似度退化为点积。个人 NAS 规模（< 100K 资产）下搜索耗时 < 50ms。决策详见 [tech-stack.md](./tech-stack.md)。

**写入链路**：资产索引 → `EventAssetIndexed` → `ai_embed` 作业 → 调用 Python sidecar `POST /embed` → `Store.IndexEmbedding()` 持久化 BLOB。

**查询链路**：
- 语义搜索：`GET /api/dam/search?semantic=<文本>` → 文本经 sidecar 编码为 CLIP 向量 → `Store.SearchByEmbedding()` 暴力余弦排序 → 返回 top-K 结果
- 视觉相似：`GET /api/dam/search?similar=<asset_id>` → 从 `embeddings` 表取已存向量 → 同上搜索
