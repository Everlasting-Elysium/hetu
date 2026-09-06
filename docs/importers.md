# 迁移导入器（Eagle / Billfish）

一键把 **Eagle** 和 **Billfish** 的库迁移进 hetu：解析对方库结构，把资产 +
文件夹 / 标签 / 评分 / 备注 / 来源 URL 映射到 hetu 的数据模型。设计依据见
issue #57，依赖导入管线（#18）与数据模型（#1）。

> 花瓣（Huaban）无官方导出、采集数据格式不确定，本期不做；待有稳定样本后再加。

## 数据流

各来源实现同一个 [`importers.Source`](../internal/importers/model.go) 接口，
**只读**读取对方库并产出统一中间模型
[`importers.ImportItem`](../internal/importers/model.go)；
[`importers.Service`](../internal/importers/service.go) 再把每个 item 走
[`index.Indexer.IndexFile`](../internal/index/indexfile.go) 入库，并把可迁移的
元数据映射到 hetu 的 folders / tags / rating / annotations。

```
Eagle/.library  ─┐
                 ├─▶ Source.Each ─▶ ImportItem ─▶ Service ─▶ index.IndexFile ─▶ assets
Billfish/.bf/*  ─┘                                   └─▶ folders/tags/rating/annotations
```

- 中间模型字段与来源无关，来源特有、hetu 无对应的字段被丢弃并记 `slog`。
- 关键正确性点：`UpsertAsset` 的 `ON CONFLICT` 不更新 `id`，故 `IndexFile`
  用自然键（owner, provider, storage_path）经
  [`Store.GetAssetByPath`](../internal/kernel/store.go) 重新取回 canonical 资产，
  再挂标签 / 文件夹 / 评分 / 注解——保证重复导入幂等。

## 放置模式（`importers.Mode`）

定义见 [service.go](../internal/importers/service.go)：

| 模式 | 行为 | 存储 provider | 迁移可用 |
|------|------|---------------|----------|
| `index` | 原地登记，不搬运（默认） | `fs`（绝对路径，见 [storage/fs](../internal/storage/fs/fs.go)） | ✅ |
| `copy` | 复制进库目录再索引 | `local`（库内相对路径） | ✅ |
| `move` | 移动进库目录再索引，删除原文件 | `local` | ❌ 迁移禁用（源须只读） |

`fs` provider 以固定名注册（见 [app.go](../internal/app/app.go)），重启后
`provider="fs"` 的资产仍可解析；它**不作为 NAS 浏览目标**，只打开 hetu 已索引的
路径。`move` 会删除源文件，违反“绝不修改原库”，故对迁移来源
（[batch.go](../internal/importers/batch.go)）强制拒绝。

## 冲突处理（`importers.Conflict`）

- `keep-both`（默认）：始终导入；同路径重复导入由自然键 upsert 天然幂等。
- `skip`：按内容哈希查
  [`Store.ListAssetsByHash`](../internal/kernel/store.go)，已存在则跳过（同一图片
  从不同库/路径重复进入时去重）。

## 字段映射

hetu 侧目标类型见 [domain](../internal/domain)；注解分层规则（manual > ai >
extracted）见 [ai-and-3d.md](./ai-and-3d.md)。备注与来源 URL 都是**机器导入**数据，
按分层规则写入 **extracted 层**，绝不进入用户手动层：备注用 `caption` 键
（`domain.KeyCaption`，会成为 FTS 描述、可被搜索；因低于 manual/ai 层，**不会覆盖**
用户在 hetu 内写的 caption），来源 URL 用 `source.url` 键（`importers.keySourceURL`）。

**created_at**：默认取来源创建时间（Eagle `btime` / Billfish `create_time`），经
`Entry.ModTime` 传入。但图片若含 EXIF 拍摄时间，索引管线
（[index/metadata](../internal/store/metadata.go)）会以 EXIF 拍摄时间覆盖
`created_at`——对照片而言这通常比“加入对方库的时间”更准确，属预期行为。

### Eagle

解析 `images/<id>.info/metadata.json`（每项）与库根 `metadata.json`（文件夹树）；
实现见 [eagle.go](../internal/importers/eagle/eagle.go)。

| Eagle 字段 | hetu 目标 | 说明 |
|------------|-----------|------|
| `name`+`ext` | `asset.name` / 原文件 | 原文件位于同目录 `<name>.<ext>` |
| `btime`（毫秒） | `asset.created_at` | `time.UnixMilli` 转秒 |
| `star` | `asset.rating` | 0–5，越界 clamp |
| `tags[]` | tags | **扁平**标签（Eagle 无层级；`/` 是名字的一部分） |
| `folders[]` | folder | 经库 `metadata.json` 文件夹树把 id 解析为路径；取首个为主文件夹 |
| `annotation` | extracted `caption` | 备注 |
| `url` | extracted `source.url` | 来源 URL |
| `isDeleted=true` | —（跳过） | 已删除项不导入 |

### Billfish

只读打开 `.bf/billfish.db`（`mode=ro&immutable=1`，绝不写源库）；实现见
[billfish.go](../internal/importers/billfish/billfish.go) 与
[billfish_query.go](../internal/importers/billfish/billfish_query.go)。自动探测
`bf_tag_v2`/`bf_tag`、`bf_material_v2`/`bf_material` 以兼容版本差异。

| Billfish 来源 | hetu 目标 | 说明 |
|---------------|-----------|------|
| `bf_file.name`+`ext` | `asset.name` / 原文件 | `path` 相对库根解析（绝对路径原样用） |
| `bf_file.create_time`（秒） | `asset.created_at` | |
| `bf_material_userdata.score` | `asset.rating` | 0–5 |
| `bf_tag_v2` + `bf_tag_join_file` | tags | 经 `pid` 还原层级路径 |
| `bf_file.pid` → `bf_folder` | folder | 单一文件夹归属，经 `pid` 还原路径 |
| `bf_material_userdata.note` | extracted `caption` | 备注 |
| `bf_material_userdata.origin` | extracted `source.url` | 来源 URL |
| `bf_material*.is_recycle=1` | —（跳过） | 回收站项不导入 |

> Billfish 只读打开假定对方 App **已关闭**（`immutable=1` 会忽略未提交的 WAL）；
> 兼容自动探测 v2/v1 表名，以 v2 为主要测试目标。

## 已知限制（尽力映射，记录未映射项）

- **标签唯一性**：hetu 标签在 owner 内按**名字**唯一（见
  [schema.sql](../internal/store/schema.sql) `idx_tags_owner_name`），不同分支下的
  同名叶子标签会**合并**；层级 `parent_id` 尽力保留。
- **单一文件夹**：`asset.folder_id` 只允许一个，来源的多文件夹归属取**首个**为主，
  其余丢弃。
- 缩略图、色板、pHash、EXIF 等由 `IndexFile` 走既有索引管线生成，不从来源迁移。
- **元数据尽力而为**：资产入库后，单个字段映射失败（如并发导入 + AI 打标同时新建
  同名标签触发唯一约束）只记 `slog` 警告并跳过，不使整条记录失败。顺序迁移
  （CLI / 同步 API）不触发此并发竞争。

## 使用

CLI（同步，见 [cli/import.go](../internal/cli/import.go)）：

```
bin/hetu import eagle    /path/to/MyLib.library
bin/hetu import billfish /path/to/MyBillfishLib      # 含 .bf/billfish.db
# --mode index|copy   --conflict keep-both|skip
```

HTTP（见 [plugins/dam/import.go](../internal/plugins/dam/import.go)）：

- `POST /api/dam/import`：导入单个松散文件（JSON `{mode,path,dest_subdir,conflict}`
  或 multipart 上传）。
- `POST /api/dam/import/migrate`：迁移库（JSON `{source,path,mode,conflict,async}`）；
  `async=true` 走 JobQueue 返回 `job_id`，进度写入 jobs 表 payload。
- `GET /api/dam/jobs`：查后台任务与迁移进度。
- `GET /api/dam/assets/{id}/raw`：按资产自身 provider 取原文件（支持 Range），
  故 `fs` 原地索引的资产也可下载。
