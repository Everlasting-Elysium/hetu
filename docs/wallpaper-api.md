# wallpaper 插件 API

wallpaper 插件把 DAM 已索引的资产以**公开、匿名、只读**的方式暴露为壁纸画廊：
筛选 / 排序 / 随机 / 每日一图 / 合集浏览 + 强制下载 + zip 打包。所有端点定义在
`internal/plugins/wallpaper/` 中。

**默认不启用**：`HETU_PLUGINS` 的默认值是 `dam,nas`
（见 [`internal/config/config.go`](../internal/config/config.go) 的 `Plugins` 字段），
必须显式设置 `HETU_PLUGINS=dam,nas,wallpaper` 才会挂载这些端点。一旦启用即为公开
——内核全局只有一层 `Access-Control-Allow-Origin: *` 的 CORS，没有任何鉴权中间件
（见 [`internal/api/server.go`](../internal/api/server.go)），这是 issue #114 明确的设计。

零迁移：本插件不新增任何数据库表，只读 DAM 建立的 `assets` / `collections` /
`collection_items` 表。

---

## 通用约定

- 所有端点都是 `GET`，挂载在 `/api/wallpaper/` 下。
- **`kind` 默认值**：筛选类端点（`/list` `/random` `/daily`）在 `?kind=` 缺省或全部非法时，
  默认只返回 `image,video`（不是 DAM 的“空 = 不限”），杜绝 audio/document/3D 混入画廊。
  该默认值定义在 [`filter.go`](../internal/plugins/wallpaper/filter.go) 的 `parseWallpaperFilter`。
- **响应 DTO**（`wallpaperDTO`，见 [`dto.go`](../internal/plugins/wallpaper/dto.go)）只含
  `id/kind/width/height/size/rating/thumb_url/download_url`，**故意不暴露**
  `storage_path/provider/folder_id/name` 等内部路径线索。
- 分页：`?limit=`（默认 `defaultLimit`，上限 `maxLimit`）、`?offset=`（下限 0）。
  常量见 [`filter.go`](../internal/plugins/wallpaper/filter.go)（当前 `defaultLimit=30`、`maxLimit=100`）。

### 共享筛选参数

`/list` `/random` `/daily` 共用 `parseWallpaperFilter`，接受：

| 参数 | 位置 | 说明 |
|------|------|------|
| `kind` | query | 逗号分隔的 AssetKind 白名单；缺省/全非法 → `image,video` |
| `shape` | query | 逗号分隔的 `landscape`/`portrait`/`square`（宽高比桶） |
| `minWidth` / `maxWidth` | query | 像素宽度范围（当前版本解析），0 表示该侧不限 |
| `minHeight` / `maxHeight` | query | 像素高度范围，0 表示该侧不限 |
| `rating` | query | 最低星级（0–5），映射到 `MinRating` |
| `collection` | query | 限定为某个合集的成员（asset 属于该合集） |
| `sort` | query | `latest`（默认）/ `rating` / `random`；非法值忽略，回退到 `latest` |

非法枚举值（`kind`/`shape`/`sort`）一律被静默丢弃，绝不 500 —— 白名单在
[`domain`](../internal/domain/) 的 `ParseKinds`/`ParseShapes`/`ValidSort` 中，
是注入防护的唯一入口。

`sort` 的三个取值定义在 [`internal/domain/sort.go`](../internal/domain/sort.go)
（`SortLatest`/`SortRating`/`SortRandom`），SQL 侧的 `ORDER BY` 片段由
[`internal/store/sqlite_filter.go`](../internal/store/sqlite_filter.go) 的
`orderByClause` 从固定白名单映射（`latest`→`indexed_at DESC`、`rating`→
`rating DESC, indexed_at DESC`、`random`→`RANDOM()`）。

---

## 端点

### GET /api/wallpaper/list

按筛选/排序分页列出壁纸。接受上方全部共享筛选参数 + `limit`/`offset`。

**响应** `200 OK`：

```json
[
  {
    "id": "019...",
    "kind": "image",
    "width": 1920,
    "height": 1080,
    "size": 1048576,
    "rating": 5,
    "thumb_url": "/api/wallpaper/019.../thumb",
    "download_url": "/api/wallpaper/019.../download"
  }
]
```

无匹配时返回空数组 `[]`（不是 `null`）。

实现：[`list.go`](../internal/plugins/wallpaper/list.go) 的 `list` 方法。

---

### GET /api/wallpaper/random

随机返回若干壁纸。除共享筛选参数外接受：

| 参数 | 位置 | 说明 |
|------|------|------|
| `count` | query | 返回数量，默认 `defaultRandomCount`（1），上限 `maxRandomCount`（50） |

`count` 超过库存时返回全部（不报错）。排序被强制为 `random`，忽略传入的 `sort`。
常量见 [`random.go`](../internal/plugins/wallpaper/random.go)。响应形状同 `/list`。

实现：[`random.go`](../internal/plugins/wallpaper/random.go) 的 `random` 方法。

---

### GET /api/wallpaper/daily

每日一图：同一自然日（`Asia/Shanghai`）内确定性地返回同一张壁纸，跨天轮换。
接受全部共享筛选参数（`sort` 被强制为 `latest` 以保证顺序确定）。

**算法**（非 `RANDOM()`，可复现）：
1. `count = CountAssetsFiltered(filter)`，为 0 → `404`。
2. `seed = 年*10000 + 月*100 + 日`（十进制日期数，如 `20260912`）。
3. `idx = seed % count`，取 `latest` 排序下 `offset=idx, limit=1` 的那一张。

时区加载失败时回退到固定 UTC+8（`time.FixedZone("CST", 8*3600)`），不会 500。

**响应** `200 OK`：单个 `wallpaperDTO` 对象（非数组）。

**错误码**：`404` — 无任何匹配资产。

实现：[`daily.go`](../internal/plugins/wallpaper/daily.go) 的 `daily` 方法；总数来自
[`internal/store/sqlite_wallpaper.go`](../internal/store/sqlite_wallpaper.go) 的 `CountAssetsFiltered`。

---

### GET /api/wallpaper/collections

列出全部合集及其封面缩略图 URL。合集封面（显式覆盖，否则最低 `ord` 成员）由 store
解析进 `Collection.Cover`，本端点仅把该 asset id 转成 `/thumb` URL；封面资产不存在或无缩略图时省略 `cover_url`。

**响应** `200 OK`：

```json
[
  { "id": "c1", "name": "风景", "cover_url": "/api/wallpaper/019.../thumb" },
  { "id": "c2", "name": "抽象" }
]
```

实现：[`collections.go`](../internal/plugins/wallpaper/collections.go) 的 `listCollections` 方法。

---

### GET /api/wallpaper/collections/{id}

按合集手工顺序（`ord`）分页列出成员壁纸。**不叠加** `kind`/`shape` 等筛选
——合集本身即已策展集合。接受 `limit`/`offset`。

| 参数 | 位置 | 说明 |
|------|------|------|
| `id` | path | 合集 id |

**响应** `200 OK`：`wallpaperDTO` 数组，按 `ord` 升序。

**错误码**：`404` — 合集不存在或不属于该 owner。

实现：[`collections.go`](../internal/plugins/wallpaper/collections.go) 的 `getCollectionAssets` 方法；
数据来自 [`internal/store/sqlite_wallpaper.go`](../internal/store/sqlite_wallpaper.go) 的
`ListCollectionAssets`（返回完整 `domain.Asset`，含 width/height/size）。

---

### GET /api/wallpaper/{id}/thumb

流式返回预生成的缩略图（`GetAsset` 的 `ThumbPath` 已是当前版本解析结果，无需版本查找）。

**响应头**：`Cache-Control: public, max-age=86400, immutable`。

**错误码**：`404` — 资产不存在或无缩略图。

实现：[`media.go`](../internal/plugins/wallpaper/media.go) 的 `serveThumb` 方法。

---

### GET /api/wallpaper/{id}/download

**强制下载**原始字节。与 DAM 的 `/file`（`inline`）不同，本端点使用
`Content-Disposition: attachment`。provider- 与 version-aware：版本化资产下载其当前版本
（通过 `currentFile` 经版本表解析 provider/storage_path），文件名取 `DisplayName`（为空则 `Name`）。

**响应头**：
- `Content-Disposition: attachment; filename="<display or name>"`
- `Cache-Control: private, max-age=3600`
- 支持 HTTP Range（`http.ServeContent`），Range 请求返回 `206 Partial Content`

**错误码**：`404` — 资产不存在 / 后端文件缺失；`400` — 路径指向目录。

实现：[`media.go`](../internal/plugins/wallpaper/media.go) 的 `download` / `currentFile` 方法。

---

### GET /api/wallpaper/download/zip

把多个壁纸打包成一个 zip 流式下载。

| 参数 | 位置 | 说明 |
|------|------|------|
| `ids` | query | 逗号分隔的 asset id 列表，按出现顺序去重 |

- 上限 `maxZipItems`（当前 50，见 [`zip.go`](../internal/plugins/wallpaper/zip.go)）。
- 解析失败/未找到/provider 未注册的 id 被静默跳过；下载中单个文件打开失败也跳过（不中断整体）。
- entry 名用 `DisplayName`（为空则 `Name`），同名冲突时后者加 ` (2)`、` (3)` 序号（插在扩展名前）。

**响应头**：`Content-Type: application/zip`、`Content-Disposition: attachment; filename="wallpapers.zip"`。

**错误码**：
- `400` — `ids` 为空 / 超过 `maxZipItems`
- `404` — 所有 id 均无法解析（结果集为空）

实现：[`zip.go`](../internal/plugins/wallpaper/zip.go) 的 `downloadZip` 方法，使用标准库 `archive/zip` 流式写出。

---

## 数据存储

wallpaper 插件**零迁移**：不新增任何表，读的是 DAM 建立的同一份数据：

- `assets` —— 通过 `ListAssetsFiltered` / `CountAssetsFiltered` / `GetAsset` 读取（表结构见 [`data-model.md`](./data-model.md)）。
- `collections` / `collection_items` —— 通过 `ListCollections` / `ListCollectionAssets` 读取（issue #55 建立）。
- 当前版本解析读 `asset_versions`（issue #58）。

新增的持久化方法（`CountAssetsFiltered` / `ListCollectionAssets` / `orderByClause`）在
[`internal/store/sqlite_filter.go`](../internal/store/sqlite_filter.go) 与
[`internal/store/sqlite_wallpaper.go`](../internal/store/sqlite_wallpaper.go)；
筛选值对象 `domain.AssetFilter` 的 `Sort`/`CollectionID` 字段见
[`internal/domain/filter.go`](../internal/domain/filter.go)。
