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

### shares

分享链接，支持过期时间、密码保护、只读权限。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID v7) | 主键 |
| target_type | TEXT | 分享目标类型：`asset` / `folder` / `tag` |
| target_id | TEXT | 分享目标的 ID |
| token | TEXT | URL 中的分享令牌（唯一） |
| expires_at | DATETIME | 过期时间，NULL 表示永不过期 |
| password_hash | TEXT | 密码哈希，NULL 表示无密码 |
| permission | TEXT | 权限：`read`（只读） |

---

### jobs

后台任务队列持久化表，用于缩略图生成、AI 打标等异步任务。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID v7) | 主键 |
| type | TEXT | 任务类型（如 `thumbnail`、`ai_tag`、`3d_render`） |
| status | TEXT | 状态：`pending` / `running` / `done` / `failed` |
| payload | TEXT | JSON 序列化的任务参数 |
| created_at | DATETIME | 入队时间 |

---

## Phase 1 表

### assets_fts（FTS5 虚拟表，已实现）

SQLite FTS5 全文检索虚拟表，为**工作区级**全文检索提供支撑（跨文件夹按文件名/标签/描述检索）。建表语句与同步触发器定义在 `internal/store/schema.sql`。

**表结构**（内容无关 / contentless，`content=''`，`tokenize='unicode61'` 支持中文）：

| 列 | 说明 |
|------|------|
| name | 资产文件名，来自 `assets.name` |
| tags | 空格分隔的标签文本；`tags`/`asset_tags` 表落地前恒为空 |
| description | 资产描述；`annotations` 表落地前恒为空 |

**同步机制**：`assets` 表的 `INSERT`/`UPDATE`/`DELETE` 由三个触发器（`trg_assets_ai`/`trg_assets_au`/`trg_assets_ad`）镜像到 `assets_fts`，`rowid` 与 `assets.rowid` 对齐。contentless 表的删除用 FTS5 专用命令 `INSERT INTO assets_fts(assets_fts, rowid, ...) VALUES('delete', ...)`。

**查询链路**：
- 解析器 `internal/search/parser.go` 把用户查询（`name:` `tag:` `desc:` 字段限定 + `AND`/`OR`/`NOT` 布尔 + 引号短语）转成参数化的 FTS5 `MATCH` 表达式，字段白名单 + 值全部加引号转义防注入；空/纯操作符查询返回 `ErrEmptyQuery`。
- 存储层 `internal/store/sqlite.go` 的 `SearchAssets` 手写 SQL（sqlc 不支持 FTS5 虚拟表），`JOIN assets` 后按 `assets_fts.rank`（bm25）相关度升序返回；非法 MATCH 表达式映射为 `domain.ErrInvalidQuery`。
- HTTP 接口 `GET /api/dam/search?q=`（`internal/plugins/dam/search.go`），空查询/非法查询返回 400，`limit` 限制在 `[1,200]`。

**升级兼容**：`assets_fts` 与触发器由 `schema.sql` 每次 `Open` 幂等重建；旧库（FTS 之前创建的）通过 `PRAGMA user_version` 门控的一次性回填（`backfillFTS`）补索引，避免旧行在下次 upsert 时触发 contentless delete 导致 `SQLITE_CORRUPT`。变更 FTS 结构（如换 tokenizer）时递增 `ftsSchemaVersion` 并补迁移。

**已知限制 / 后续（本 issue 范围外）**：
- **CJK 分词**：`unicode61` 把连续中文当作单个 token，无法子串匹配（如「海滩」搜不到「日落海滩风景」）。待 `tags`/`description` 落地且有真实中文数据后，评估切换 `trigram`（支持子串，但需 ≥3 字符、改变英文为子串匹配与排序语义）或 ICU。切换需借 `user_version` 做 FTS 重建。
- **并发**：默认 journal 模式下 `hetu serve`（读）与 `hetu scan`（写，含 FTS 触发器）并发可能 `SQLITE_BUSY`；后续考虑 WAL + `busy_timeout`。
- **多词字段值**：`name:日落 海滩` 中字段限定只作用于第一个词（`海滩` 退化为全列词），需要多词请用引号：`name:"日落 海滩"`。

### embeddings（sqlite-vec）

存储 CLIP 向量嵌入，用于语义搜索和视觉相似度。

| 字段 | 类型 | 说明 |
|------|------|------|
| asset_id | TEXT | 外键 → assets.id，主键 |
| embedding | BLOB | 向量数据（float32 数组） |
| model | TEXT | 生成该嵌入的模型标识 |
| created_at | DATETIME | 写入时间 |

**注意**：此表依赖 sqlite-vec C 扩展，存在 CGO 兼容性开放问题，详见 [tech-stack.md](./tech-stack.md)。
