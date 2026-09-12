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
| kind | TEXT | 资产类型（image / video / audio / model / document / font / design / other），完整枚举与显示顺序见 [domain.AllKinds](../internal/domain/asset.go) |
| name | TEXT | 文件名（不含扩展名） |
| ext | TEXT | 扩展名（小写，含点，如 `.jpg`） |
| size | INTEGER | 文件字节数 |
| hash | TEXT | 文件内容哈希（SHA-256），用于去重检测 |
| thumb_path | TEXT | 缩略图存储路径，可为空 |
| width | INTEGER | 图片/视频宽度（像素），非图像类型为 NULL |
| height | INTEGER | 图片/视频高度（像素），非图像类型为 NULL |
| created_at | DATETIME | 文件创建时间（来自文件系统或 EXIF） |
| indexed_at | DATETIME | hetu 最后一次索引该文件的时间 |
| rating | INTEGER | 用户评分，0–5 星，默认 0 |
| color | TEXT | 用户颜色标签（如 `#FF5733`），空串表示无 |
| favorite | INTEGER | 用户收藏标记（issue #62），DB 存 0/1，默认 0；Go 侧映射为 `bool`（`domain.Asset.Favorite`），转换见 `internal/store/sqlite.go` 的 `boolToInt64` / `rowToAsset` |
| current_version_id | TEXT | 当前版本指针 → `asset_versions.id`；空字符串表示无显式版本（锚点行自身即唯一隐式版本），详见 [asset_versions](#asset_versions) |
| palette_manual | INTEGER | 手动调色板标记（issue #62），DB 存 0/1，默认 0；为 1 表示该资产的 `asset_colors` 已被人工整理，`writePaletteTx` 在重新扫描/缩略图重提取时整体跳过、不覆盖。风格比照 `favorite`（0/1 INTEGER + `migrate.go` 旧库迁移），详见 [asset_colors](#asset_colors) |

> `rating` / `color` / `favorite` / `display_name` / `folder_id` 及回收站/丢失状态（`deleted_at` / `missing_at`）均为**用户元数据**：`UpsertAsset` 仅在首次插入时写入，重新扫描经 `ON CONFLICT` 只更新索引派生字段（kind/name/size/hash/缩略等），因此再次扫描不会覆盖用户的评分/颜色/收藏。完整列见 [schema.sql](../internal/store/schema.sql)。

**版本解析（issue #58）**：`storage_path` / `hash` 始终锚定最初被索引的原始文件（扫描、去重、relocate 均以其为准，因此版本功能不影响这些链路）。而读取接口（`GetAsset` / `ListAssets` / `ListAssetsFiltered` / `SearchAssets`）通过 `LEFT JOIN asset_versions` + `COALESCE` 将 `thumb_path` / `width` / `height` 解析为**当前版本**的值，使缩略/搜索反映当前版本，无需把版本数据写回 `assets`（否则会被下次扫描的 `UpsertAsset` 覆盖）。

**富格式 kind（[#48](https://github.com/Everlasting-Elysium/hetu/issues/48)）**：三类富格式资产的语义与降级策略——外部工具/内嵌数据缺失时一律仍正常入库，仅缺预览：

- `psd`（Photoshop）：本质是图片，归 **`kind=image`**（复用图片的全部下游筛选/画板/调色板逻辑，不新建 kind），用纯 Go `oov/psd` 读取**合成图（merged image）**出缩略图；hetu 不做图层混合，无合成图数据则 `ErrNoThumbnail`。元数据写入 `psd.layer_count` / `psd.color_mode` / `psd.bit_depth`（extracted 层）。处理器：[internal/asset/psd/](../internal/asset/psd/)。
- `font`（ttf/otf/woff）：字体文件，`kind=font`，无宽高。预览为处理器用该字体**自渲染的字形样张**（拉丁 + 数字；仅当字体含 CJK 字形时附一行中文，不用回退字体）。元数据从 name table 读 `font.family` / `font.weight` / `font.style`（extracted 层）。woff2 不匹配（`opentype` 解码器不支持其 Brotli 容器）。处理器：[internal/asset/font/](../internal/asset/font/)。
- `design`（ai/indd/sketch/fig/aep）：不透明设计文件，一律 `kind=design` 且不解析尺寸；仅部分可出图——PDF 兼容的 `.ai` 经共享的 `document.PDFPageRenderer` 出首页图、`.sketch`（zip）提取内嵌 `previews/preview.png`，其余（indd/fig/aep 及非 PDF 的 ai）仅登记不出图。格式按内容 magic bytes 判别（处理器方法只拿到 reader，与专业图片处理器一致）。处理器：[internal/asset/design/](../internal/asset/design/)。

---

### folders

文件夹层级结构，用于 DAM 插件的虚拟组织（与文件系统路径解耦）。封面/颜色见 [#62](https://github.com/Everlasting-Elysium/hetu/issues/62)，DDL 与实现见 [schema.sql](../internal/store/schema.sql)、[queries/folder.sql](../internal/store/queries/folder.sql)、[store/sqlite_folders.go](../internal/store/sqlite_folders.go)、[plugins/dam/folders.go](../internal/plugins/dam/folders.go)。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID v7) | 主键 |
| owner_id | TEXT | 外键 → users.id |
| parent_id | TEXT | 外键 → folders.id，根文件夹为空字符串（`NOT NULL DEFAULT ''`） |
| name | TEXT | 文件夹名称 |
| path | TEXT | 从根到当前节点的完整路径（冗余存储，加速查询） |
| cover | TEXT | 可选封面 asset_id 覆盖；空字符串表示自动取该文件夹最早入库的资产 |
| color | TEXT | 颜色标识（如 `#FF5733`，风格对齐 `tags.color`），可为空 |

唯一索引：`idx_folders_owner_path`（`owner_id, path`）。

**有效封面解析**：`ListFolders`（经单条 `ListFoldersWithCover` query，用 `CASE`/子查询一次性算出，避免逐文件夹 N+1）返回**有效封面**——`cover` 非空则用它，否则回退到该文件夹下（`assets.folder_id = folders.id AND deleted_at IS NULL`）按 `indexed_at ASC` 排序的第一个（即最早入库）资产，文件夹为空则为空字符串；`GetFolder` 则返回**原始** `cover`，编辑路径据此区分显式覆盖与自动回退，不会把回退值误存为显式覆盖。`cover` 经 `UpdateFolderCover` 设置时在 store 层校验其为当前 owner 名下真实存在且未软删除的资产，否则返回 `domain.ErrNotFound`（防止挂空引用或跨 owner 引用 IDOR）。HTTP 层（`GET /api/dam/folders`）把有效封面 asset_id 解析为 `/api/dam/assets/{id}/thumb` 形式的 `cover_url`（资产不存在或无缩略图时省略），前端侧边栏文件夹行据此显示封面缩略图与颜色点。旧库经 [migrate.go](../internal/store/migrate.go) 的 `ALTER TABLE folders ADD COLUMN` 补齐 `cover`/`color` 两列。

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

联合主键：`(asset_id, tag_id)`——天然保证一个资产对同一标签至多一行，是下述合并/替换去重语义的基础。

**标签合并 / 批量替换（[#62](https://github.com/Everlasting-Elysium/hetu/issues/62)）**：两个互补且独立的操作，DDL/实现见 [queries/tag.sql](../internal/store/queries/tag.sql)、[store/sqlite_tags.go](../internal/store/sqlite_tags.go)、[plugins/dam/tags.go](../internal/plugins/dam/tags.go)、[plugins/dam/batch.go](../internal/plugins/dam/batch.go)。

- **合并（全局）**：`MergeTags` 把标签 A 折进标签 B 并删除 A，**作用于全部素材**（非选中集）。事务内一次完成：① 用 `INSERT OR IGNORE ... SELECT`（`ReattachAssetTags`）把 `tag_id = A` 的资产改挂到 B，`(asset_id, tag_id)` 主键天然去重——同时带 A、B 的资产合并后只保留一条 B 行，不会因主键冲突报错；② `DeleteAssetTagsByTag` 删除残余的 A 行；③ 删除 `tags` 表里的 A。校验：A、B 都须存在且属于同一 owner（否则 `domain.ErrNotFound`→404），且不得相同（否则 `domain.ErrSameTag`→400，防止把标签合并进自己）。**子标签策略**：A 的直接子标签（`tags.parent_id = A`）在删除 A 前经 `ReparentTagChildren` **上提一级**到 A 原来的父级（顶级标签则提为顶级），避免留下指向已删除标签的悬空 `parent_id`；该策略对「合并目标 B 本身就是 A 的子标签」这一棘手情形同样安全——B 会像其他子标签一样被上提，绝不会指向已删除的 A 或指向自己。端点 `POST /api/dam/tags/merge`，body `{from_tag_id, to_tag_id}`；破坏性且不可逆，前端以确认弹窗兜底（拖拽标签到目标标签触发）。
- **批量替换（子集）**：`BatchReplaceTag` 只在**给定 `asset_ids` 子集**内把标签 X 换成 Y，**不删除标签 X 本身**（选中集之外的资产可能仍在用）。事务内：① `ReattachAssetTagsForAssets`（`INSERT OR IGNORE ... SELECT ... WHERE tag_id = X AND asset_id IN (...)`）把子集内 X 改挂到 Y，同一 `(asset_id, tag_id)` 主键去重；② 复用 `BatchRemoveTags` 删除子集内的 X 行。X == Y 时报 `domain.ErrSameTag`→400（否则「自复制后再删」会误把该标签从选中资产上抹掉）。端点 `POST /api/dam/batch/replace-tag`，body `{asset_ids, from_tag_id, to_tag_id}`，写法/错误处理对齐 `/batch/rate`、`/batch/color`、`/batch/favorite`。

> 两者的 `INSERT OR IGNORE` + 范围内 `DELETE` 去重语义一致，唯一区别是**作用域**（全局 vs 选中集）与**是否删除源标签**（合并删、替换留）。`asset_tags` 的增删经 `trg_asset_tags_ai`/`trg_asset_tags_ad` 触发器自动同步到 FTS 的 `tags` 列，故合并/替换后全文检索无需额外处理。

---

### collections

手动分组（[#55](https://github.com/Everlasting-Elysium/hetu/issues/55)），独立于文件夹树的用户自定义集合。可嵌套子合集（`parent_id` 自引用），一个资产可归属多个合集，成员支持手动排序。DDL 与实现见 [schema.sql](../internal/store/schema.sql)、[queries/collection.sql](../internal/store/queries/collection.sql)、[store/sqlite_collections.go](../internal/store/sqlite_collections.go)、[plugins/dam/collections.go](../internal/plugins/dam/collections.go)。

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT (UUID v7) | 主键 |
| owner_id | TEXT | 外键 → users.id |
| parent_id | TEXT | 外键 → collections.id，顶级合集为空字符串（`NOT NULL DEFAULT ''`） |
| name | TEXT | 合集名称。**不加唯一约束**：允许同名、允许不同父下重名（区别于 folders/tags） |
| cover | TEXT | 可选封面 asset_id 覆盖；空字符串表示自动取 `ord` 最小成员 |

索引：`idx_collections_owner`、`idx_collections_owner_parent`。

**有效封面解析**：`ListCollections`（经单条 `ListCollectionsWithCover` query，用 `CASE` 一次性算出，避免逐合集 N+1）返回**有效封面**——`cover` 非空则用它，否则回退到 `ord` 最小成员的 asset_id，合集为空则为空字符串；`GetCollection` 则返回**原始** `cover`，编辑路径据此区分显式覆盖与自动回退，不会把回退值误存为显式覆盖。`cover` 经 `UpdateCollection` 设置时在 store 层校验其为当前成员，非成员返回 `domain.ErrNotFound`。

---

### collection_items

合集的有序成员关系。`ord` 决定成员在合集内的手动排序（升序），联合主键保证同一资产在一个合集内至多一条。实现见 [store/sqlite_collection_items.go](../internal/store/sqlite_collection_items.go)、[plugins/dam/collection_items.go](../internal/plugins/dam/collection_items.go)。

| 字段 | 类型 | 说明 |
|------|------|------|
| collection_id | TEXT | 外键 → collections.id |
| asset_id | TEXT | 外键 → assets.id |
| ord | INTEGER | 合集内排序位（升序，`NOT NULL DEFAULT 0`） |

联合主键：`(collection_id, asset_id)`；`idx_collection_items_collection` 覆盖 `(collection_id, ord)` 排序。

**增删 / 排序语义**：
- **追加**：`AddCollectionItem` 在同一事务内取 `MAX(ord)+1` 再 upsert，`ON CONFLICT(collection_id, asset_id) DO UPDATE SET ord` 保证幂等（重复 add 同一资产更新 `ord` 而非报错、不产生重复行）。
- **重排**：`ReorderCollectionItems` 事务内按传入数组下标重写全部 `ord`（`0..n-1`），先校验数组与当前成员**完全一致**（数量、成员相同、无重复），不一致返回 `domain.ErrCollectionItemsMismatch`（HTTP 400）。
- **删除合集级联**：`DeleteCollection` 在事务内先清空 `collection_items` 再删 `collections`，不留孤儿行（区别于 folders/tags 对关联表不做显式清理）。
- **成员富化**：`ListCollectionItems` 用单条 `ListCollectionItemsEnriched`（JOIN `assets`）返回成员的 kind/name/thumb，一次查询而非 N+1。

---

### 组织方式边界：folder / tag / collection / smart-folder

DAM 提供多种正交的资产组织方式，四者边界清晰，实现者据此选择或扩展：

| 方式 | 归属 | 结构 | 成员排序 | 状态 | 表 |
|------|------|------|----------|------|------|
| **folder（文件夹）** | 单归属（一个资产恰属一个文件夹） | 嵌套树 + 物理路径（`path` 冗余存储） | 可指定封面 + 颜色（[#62](https://github.com/Everlasting-Elysium/hetu/issues/62)） | 已实现 | `folders` + `assets.folder_id` |
| **tag（标签）** | 多对多（一个资产多个标签） | 扁平（`parent_id` 仅用于组织标签自身） | 无 | 已实现 | `tags` + `asset_tags` |
| **collection（合集，[#55](https://github.com/Everlasting-Elysium/hetu/issues/55)）** | 多对多（一个资产多个合集） | 嵌套树（`parent_id`） | 手动排序（`ord`）+ 可指定封面 | 已实现 | `collections` + `collection_items` |
| **smart-folder（智能文件夹，[#17](https://github.com/Everlasting-Elysium/hetu/issues/17)）** | 多对多（命中条件即属于） | 由保存的查询条件自动聚合，无显式成员 | 由查询决定 | **未实现** | 计划：仅存查询条件，不落地成员表 |

- **folder vs collection**：folder 是资产的**唯一物理归属**（移动即改写 `assets.folder_id`）；collection 是**叠加的手动分组**，一个资产可同时在多个合集，且成员有 `ord` 手动排序与可选封面。
- **tag vs collection**：tag 是**扁平多对多标签**（无成员排序、无封面）；collection 是**嵌套 + 有序 + 有封面**的分组。
- **collection vs smart-folder**：collection 成员**手动**增删并排序（落地到 `collection_items`）；smart-folder（[#17](https://github.com/Everlasting-Elysium/hetu/issues/17)，尚未实现）成员由**保存的查询条件自动聚合**，不落地成员表。

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

联合主键：`(asset_id, ord)`；`owner_id` 上有索引。重新索引时按 `asset_id` 整体删除后重写（`palette_manual=1` 时整体跳过，见下文手动编辑）。

检索接口：`GET /api/dam/search?color=<hex>&tol=<ΔE00>&limit=<n>`，按主色到查询色的 CIEDE2000 距离升序返回相近资产（`tol` 默认见 [search.go](../internal/plugins/dam/search.go) 的 `defaultColorTol`）。

**手动编辑调色板（issue #62）**：三个端点直接增删改 `asset_colors` 本身（不新建表、主键不变），每次编辑都在**同一事务**内把 `assets.palette_manual` 置 1：

- `POST /api/dam/assets/{id}/colors`，body `{hex}`：新增一个色卡，`ord = 当前最大 ord + 1`（空调色板则为 0）。
- `PUT /api/dam/assets/{id}/colors/{ord}`，body `{hex}`：调整某色卡的颜色，用 [internal/color](../internal/color/color.go) 的 `ParseHex` / `RGB.Lab` 重算 `l/a/b` 一并写入。
- `DELETE /api/dam/assets/{id}/colors/{ord}`：删除某色卡，删后把剩余色卡按原相对顺序重编号为连续的 `0..N-1`（不留空洞，保证 `ord=0` 恒为主色；删掉 `ord=0` 则原 `ord=1` 顶上成为新主色）。

三端点均：校验资产属主（否则 404）、拒绝不支持调色板的 kind（`domain.AssetKind.SupportsColorPalette()` 为 false，如音频 #88，返回 400 而非 500）、hex 非法返回 400，成功返回更新后的完整调色板（`[{hex,weight}]`，与 `GET .../colors` 同构）。实现见 [store/palette_edit.go](../internal/store/palette_edit.go)、[plugins/dam/colors.go](../internal/plugins/dam/colors.go)。

**非破坏保护**：`writePaletteTx`（[store/palette.go](../internal/store/palette.go)）在写入前读 `palette_manual`——为 1 时**整体跳过**（既不重写 `annotations` 的 `palette` / `dominant`，也不重写 `asset_colors`，避免出现「注释被覆盖但色卡没被覆盖」的半吊子状态），因此扫描（`IndexPalette`）与缩略图重提取（`IndexPaletteByID`，#88）都不会覆盖用户手动整理的调色板。此保护与既有的[分层元数据非破坏](./ai-and-3d.md)（manual > ai > extracted）一脉相承：用户意图（manual）优先于自动提取（extracted）。

---

### document_pages

多页文档的**逐页缩略图索引**（[#48](https://github.com/Everlasting-Elysium/hetu/issues/48)）：PDF 原生渲染，PPT/PPTX 先经 LibreOffice headless 转 PDF 再复用同一条 PDF 分页链路（不为 PPT 复制分页逻辑）。每页一行，`thumb_path` 指向缩略图目录下的 `{assetID}_p{pageNo}.jpg`。DDL 与实现见 [schema.sql](../internal/store/schema.sql)、[queries/document_page.sql](../internal/store/queries/document_page.sql)、[store/sqlite_document_pages.go](../internal/store/sqlite_document_pages.go)、[index/pages.go](../internal/index/pages.go)、处理器 [asset/document/](../internal/asset/document/)。

| 字段 | 类型 | 说明 |
|------|------|------|
| asset_id | TEXT | 外键 → assets.id |
| owner_id | TEXT | 外键 → users.id，按库主检索 |
| page_no | INTEGER | 页码（从 1 开始） |
| thumb_path | TEXT | 该页缩略图路径（`{ThumbDir}/{assetID}_p{pageNo}.jpg`） |
| width / height | INTEGER | 该页缩略图像素尺寸，`NOT NULL DEFAULT 0` |

联合主键：`(asset_id, page_no)`；`idx_document_pages_owner` 覆盖 `owner_id`。

**能力接口**：处理器实现可选接口 `kernel.PageExtractor`（`PageCount` + `RenderPage`，`page` 从 1 起）即被索引识别，写法与 `PaletteExtractor`/`MetadataExtractor` 一致（indexer 用 type assertion 检测）。document 处理器同时导出 `document.PDFPageRenderer`（仅 `RenderPage`），供 design 处理器复用其 pdftoppm/mutool 探测（.ai 出图），避免重复实现子进程逻辑。

**重建语义**：索引流水线 `indexPages`（在 upsert 之后调用、失败只 warn 不中断，仿 `indexPalette`/`indexMetadata`）逐页渲染到独立缩略图文件（单页失败跳过不放弃其余），整体经 `ReplaceDocumentPages` 替换该资产全部页行——事务内先按 asset 删旧页再批量插入新页，并按自然键 `(owner, provider, storage_path)` 解析 canonical id（与调色板/元数据写入一致，重扫生成的新 id 被 `UpsertAsset` ON CONFLICT 丢弃后仍解析到持久 id），故页数变少不残留旧行。0/1 页或工具缺失（`PageCount` 返回 `ErrNoThumbnail`）时清空页行——单张主缩略图已覆盖，不阻断入库。

**HTTP 接口**（[internal/plugins/dam/document_pages.go](../internal/plugins/dam/document_pages.go)）：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/dam/assets/{id}/pages` | 列出各页 `{page_no, thumb_url, width, height}`（按页码升序；无页返回 `[]`；资产不存在/跨库主 404） |
| GET | `/api/dam/assets/{id}/pages/{pageNo}/thumb` | 流式返回某页缩略图（JPEG，`Cache-Control: public, max-age=86400, immutable`；页/文件不存在 404） |

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

**格式 / 星级 facet（issue #75）**：`GET /api/dam/assets` 与 `GET /api/dam/search` 均接受 `?kind=<a,b>`（逗号分隔，仅 `AssetKind` 枚举值，经 `domain.ValidKind` 白名单 + 参数化 `a.kind IN (...)` 防注入），与 `?folder=`/`?tag=`/`?rating=<最低星级>` 在服务端叠加过滤（AND 组合）。`ListAssetsFiltered` 与 `SearchAssets` 共用 `appendFacetConds`（`internal/store/sqlite_filter.go`）保证两条链路语义一致，前端不再内存过滤。新增 `GET /api/dam/facets`（`internal/plugins/dam/facets.go`）返回各 `kind` 的存量计数，受 `?folder=`/`?tag=`/`?rating=` 约束但忽略 `?kind=` 自身，供多选格式 facet 稳定驱动。

**收藏 facet（issue #62）**：`GET /api/dam/assets` 与 `GET /api/dam/search` 均接受 `?favorite=true`（经 `httpjson.QueryBool` 解析，接受 `strconv.ParseBool` 形式），只保留 `assets.favorite = 1` 的资产，与 `?rating=`/`?kind=` 等同经 `appendFacetConds` 服务端叠加（AND）；**不传该参数（或传 false）时零约束**，既有列表/搜索行为不变。收藏是 `assets` 表自身的直接列（非 annotations 分层字段），因此不需要额外 JOIN。置位/取消经 `POST /api/dam/batch/favorite`（body `{asset_ids, favorite}`，`favorite=true` 收藏、`false` 取消——一个端点覆盖两个方向，与 `/batch/rate`、`/batch/color` 取值同构；单卡片收藏即以单元素 `asset_ids` 调用该端点）。

**批量导出打包（issue #62）**：`POST /api/dam/batch/export`，body `{asset_ids}`（POST + JSON body，与全部 `/batch/*` 端点同构；不同于 wallpaper 的匿名 `GET /download/zip?ids=`，也不同于 [#4](https://github.com/Everlasting-Elysium/hetu/issues/4) 的 NAS 单文件下载/分享——这里面向 DAM 选择集做批量打包）。响应 `Content-Type: application/zip`、`Content-Disposition: attachment; filename="assets.zip"`，用标准库 `archive/zip` 流式写出。每个 id 经 owner 作用域的 `Store.GetAsset` 校验，跨 owner / 不存在 / provider 未注册的 id 静默跳过而不中断；文件字节按当前版本解析（issue #58，复用 `currentVersionFile`，与 `/file` 一致），zip entry 名优先 `DisplayName`（为空则 `Name`），同名冲突加 ` (2)`/` (3)` 序号（插在扩展名前）。空 `asset_ids` 或超上限 → 400，全部无法解析 → 404；流已开始后单文件打开失败只能跳过（无法再改 HTTP 状态）。打包核心（流式写出、单项失败跳过、同名去重、上限常量 `MaxItems`）抽到共享包 [internal/ziputil](../internal/ziputil/ziputil.go) 的 `Stream`/`Item`/`MaxItems`，与 wallpaper 的 `download/zip` 共用同一份实现（wallpaper 侧 `maxZipItems` 别名到 `ziputil.MaxItems`，不重复定义魔法数字）。实现见 [export.go](../internal/plugins/dam/export.go) 的 `batchExport`。

**形状 / 尺寸 / 文件大小 facet（issue #101，#75 的直接延伸）**：在格式/星级之上再叠三维，零 DB 迁移（`assets.size`/`width`/`height` 早已入库），同样经 `appendFacetConds` 同时作用于列表与搜索。`?minSize=`/`?maxSize=`（字节，`int64`——文件可 >2GB，新增 `httpjson.QueryInt64`）narrow 在 `a.size`（**锚点值，不做当前版本解析**，与展示的 `a.size` 保持一致）；`?minWidth=`/`?maxWidth=`/`?minHeight=`/`?maxHeight=`（像素）与 `?shape=<a,b>`（逗号分隔，枚举 `landscape`/`portrait`/`square`，经 `domain.ValidShape` 白名单）narrow 在 `COALESCE(cv.width/height, a.width/height)`——即**当前版本**解析值（issue #58），与展示的宽高保持一致；「size 用锚点、width/height 走版本解析」的不对称是刻意保留，并非需要修正的不一致。形状按宽高比 `r = 宽/高` 分桶（命名常量见 `internal/domain/shape.go`）：`r ≥ 1.1` 横向、`r ≤ 0.9` 纵向、其余方形；宽或高为 `0`（音频/文档/多数 3D）不落入任何形状桶。范围参数负值按 `0`（不限）处理，`max>0 且 max<min` 时 `max` 视为不限（而非交换）。`GET /api/dam/facets` 的 `KindCounts` 补了 `LEFT JOIN asset_versions cv`（`ListAssetsFiltered`/`SearchAssets` 已有，facets 端点此前缺失）以支持新增条件联动格式计数，`cv.id` 是主键的 `LEFT JOIN` 不放大行数。

**时长 / 时间 facet（issue #53，#101 的直接延伸）**：再叠两维，同样零 DB 迁移、同样经 `appendFacetConds` 同时作用于列表与搜索。**时长**：`?minDuration=`/`?maxDuration=`（秒，`float64`——秒可为小数，新增 `httpjson.QueryFloat64`）narrow 在 `annotations` 表的 `audio.duration`/`video.duration`（extracted 层，JSON `float64` 秒）而非 `assets` 列，故 `appendFacetConds` 经 `durationJoin`（`internal/store/sqlite_filter.go` 的 `adur` 别名：`LEFT JOIN annotations ... AND "key" IN ('audio.duration','video.duration')`）取值后 `CAST(adur.value AS REAL)` 比较；两个 key 对同一资产互斥（一次只由一个 handler 处理），`LEFT JOIN` 至多一行不放大计数。视频时长由 `internal/asset/video/video.go` 的 `Handler.ExtractMetadata`（新实现 `kernel.MetadataExtractor`，仿音频 handler）在扫描时写入新增的 `domain.KeyVideoDuration = "video.duration"`；音频时长沿用既有 `audio.duration`。无时长的资产（图片/文档/多数 3D）`adur.value` 为 `NULL`，任何时长区间条件天然将其排除，无需额外 zero-guard。**时间**：`?createdAfter=`/`?createdBefore=`/`?indexedAfter=`/`?indexedBefore=`（unix 秒，用 `httpjson.QueryInt64`）直接 narrow 在 `assets.created_at`/`assets.indexed_at`（INTEGER unix 秒列），无需 JOIN。`durationJoin` 与 `cv` join 一样必须同时出现在 `ListAssetsFiltered`/`SearchAssets`/`KindCounts` 三处 SQL 拼装（否则 `adur` 别名不解析——即 #101 踩过的坑）。范围归一化沿用同一约定：负值→不限、`max>0 且 max<min` 丢弃 `max`（时长用 `normalizeRangeFloat`，时间用 `normalizeRange`，均在 `internal/plugins/dam/facets.go`）。

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
