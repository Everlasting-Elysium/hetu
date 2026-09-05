# NAS 插件 API

NAS 插件提供文件浏览、下载和分享功能。所有端点定义在 `internal/plugins/nas/` 中。

---

## 端点

### GET /api/nas/browse

列出指定路径下的文件和目录。

| 参数 | 位置 | 说明 |
|------|------|------|
| `path` | query | 存储相对路径，空值表示根目录 |

**响应** `200 OK`：

```json
[
  { "name": "photos", "path": "photos", "is_dir": true, "size": 0 },
  { "name": "readme.txt", "path": "readme.txt", "is_dir": false, "size": 1024 }
]
```

实现：[`nas.go`](../internal/plugins/nas/nas.go) 的 `browse` 方法。

---

### GET /api/nas/download

流式下载文件，支持 HTTP Range（断点续传）。

| 参数 | 位置 | 说明 |
|------|------|------|
| `path` | query | **必填**，存储相对路径 |

**响应头**：
- `Content-Disposition: attachment; filename="<name>"`
- `Accept-Ranges: bytes`
- Range 请求返回 `206 Partial Content`

**错误码**：
- `400` — 缺少 `path` 参数 / 路径指向目录
- `404` — 文件不存在

实现：[`download.go`](../internal/plugins/nas/download.go) 的 `download` 方法，底层使用 `http.ServeContent`。

---

### POST /api/nas/shares

创建分享链接。

**请求体** `application/json`：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `target_type` | string | 是 | `"file"` 或 `"folder"` |
| `target_path` | string | 是 | 存储相对路径 |
| `expires_in` | int | 否 | 过期秒数，0 或省略表示永不过期 |
| `password` | string | 否 | 访问密码，以 bcrypt 哈希存储，明文不落库 |
| `permission` | string | 否 | 权限，目前仅支持 `"read"`（默认） |

**响应** `201 Created`：

```json
{
  "id": "019...",
  "token": "a1b2c3...64位hex...",
  "url": "/s/a1b2c3...64位hex...",
  "expires_at": "2026-09-06T12:00:00Z"
}
```

`expires_at` 为 `null` 时表示永不过期。

**错误码**：
- `400` — 无效的 `target_type` / 空 `target_path` / 不支持的 `permission`

实现：[`share.go`](../internal/plugins/nas/share.go) 的 `createShare` 方法。Token 由 `crypto/rand` 生成 32 字节随机数（`tokenBytes` 常量），hex 编码为 64 字符。密码哈希使用 `golang.org/x/crypto/bcrypt`（`bcrypt.DefaultCost`）。

---

### GET /s/{token}

公开访问分享链接，无需登录。这是**顶层路由**（不在 `/api/nas/` 下），通过 `kernel.TopLevelRouter` 接口注册。

| 参数 | 位置 | 说明 |
|------|------|------|
| `token` | path | 分享令牌 |
| `password` | query | 密码保护的分享需提供 |
| `path` | query | 文件夹分享时浏览子目录（相对于分享根目录） |

**行为**：
1. 按 `token` 查询 `shares` 表
2. 检查过期：过期返回 `410 Gone`
3. 检查密码：有密码但未提供返回 `401`，密码错误返回 `403`
4. 按 `target_type` 分发：
   - `"file"` — 流式下载文件（同 `/api/nas/download`）
   - `"folder"` — 返回目录列表 JSON（同 `/api/nas/browse`）

**安全**：
- 文件夹分享的 `?path=` 参数经 `filepath.Join("/", sub)` 清理，防止 `../` 逃逸分享目录范围
- 存储层 `StorageProvider.resolve` 额外保证不逃逸存储根目录

**错误码**：
- `401` — 需要密码但未提供
- `403` — 密码错误 / 路径逃逸分享目录
- `404` — 分享不存在
- `410` — 分享已过期

实现：[`share.go`](../internal/plugins/nas/share.go) 的 `accessShare` 方法。

---

## 数据存储

分享记录持久化在 SQLite 的 `shares` 表中，表结构见 [`data-model.md`](./data-model.md#shares)。

关键字段：
- `token` — 唯一索引，URL 中的分享令牌
- `password_hash` — bcrypt 哈希，空字符串表示无密码
- `expires_at` — unix 秒，NULL 表示永不过期
- `target_id` — 存储的是文件/文件夹的存储相对路径

持久化层代码：[`internal/store/sqlite_shares.go`](../internal/store/sqlite_shares.go)（`CreateShare` / `GetShareByToken`）。
