# 路线图

## Phase 0：脊柱（最小闭环）

目标：DAM 插件 + NAS 插件的最小可用闭环，证明微内核架构可行。

### 内核与基础设施

- [ ] 微内核 `Kernel` struct，实现 `StorageRegistry`、`AssetRegistry`、`EventBus`、`JobQueue` 接口
- [ ] 环境变量配置加载（`internal/config`），`HETU_PLUGINS` 控制启用插件列表
- [ ] slog 日志初始化（`internal/obs`）
- [ ] cobra CLI 框架，`serve` 和 `scan` 子命令（`internal/cli`）
- [ ] SQLite 数据库初始化，schema 迁移（`internal/store/schema.sql`）
- [ ] `Store` 接口实现：`EnsureOwner`、`UpsertAsset`、`ListAssets`、`Close`

### 存储层

- [ ] 本地文件系统 `StorageProvider` 实现（`internal/storage/local`）
- [ ] `StorageRegistry` 注册与查找

### 资产处理

- [ ] 图片 `AssetHandler`：EXIF 元数据提取 + 纯 Go 缩略图生成（`internal/asset/image`）
- [ ] `AssetRegistry` 按扩展名路由到对应处理器

### 索引

- [ ] `Indexer`：遍历 StorageProvider → 匹配 AssetHandler → 提取元数据 → 生成缩略图 → 写入 Store（`internal/index`）
- [ ] `scan` CLI 命令触发索引

### DAM 插件（`internal/plugins/dam`）

- [ ] 文件夹 CRUD API
- [ ] 层级标签 CRUD API（含颜色）
- [ ] 资产打标 API（手动层，写入 `asset_tags` 和 `annotations`）
- [ ] 资产列表 API（按文件夹、标签过滤）
- [ ] 资产评分、颜色标记 API
- [ ] 基础关键词搜索（按文件名、标签名）

### NAS 插件（`internal/plugins/nas`）

- [ ] 文件浏览 API（列目录）
- [ ] 文件下载 API
- [ ] 分享链接创建 API（token、过期时间、密码、只读权限）
- [ ] 分享链接访问 API（无需登录，凭 token 访问）

### HTTP 服务

- [ ] chi router 组装，挂载已启用插件路由（`internal/api`）
- [ ] 基础中间件：日志、recover、CORS

### Web UI

- [ ] 最小可用前端：资产列表、标签树、文件浏览、分享链接管理

### 部署

- [ ] `Dockerfile`（多阶段构建，最终镜像基于 distroless 或 scratch）
- [ ] `deploy/docker-compose.yml`：core 服务（Phase 0 仅 core）

---

## Phase 1：AI 楔子（解决核心痛点）

目标：本地 AI 批量自动打标 + CLIP 语义搜索 + 3D 资产管理。

### AI Sidecar

- [ ] Python AI sidecar 基础框架（`ai/`），HTTP/gRPC 接口定义
- [ ] 图像打标（tagger）：本地模型推理，输出标签 + 置信度
- [ ] 图像描述（captioner）：本地模型推理，输出自然语言描述
- [ ] CLIP 嵌入：图像和文本向量化
- [ ] OCR：从图像/文档提取文字
- [ ] 人脸检测与聚类

### AI 打标流水线

- [ ] 索引完成后自动入队 AI 打标任务（EventBus → JobQueue）
- [ ] Go 内核调用 AI sidecar，将结果写入 `annotations` 表（`ai` 层）
- [ ] AI 层可单独清除并重跑（不影响 `manual` 层）

### 全文检索（FTS5）

- [ ] `assets_fts` FTS5 虚拟表，索引资产名称、标签、描述
- [ ] 搜索 API 支持全文检索

### 向量搜索（sqlite-vec，开放问题）

- [ ] 确定 sqlite-vec CGO 兼容性方案（见 [tech-stack.md](./tech-stack.md)）
- [ ] `embeddings` 表，存储 CLIP 向量嵌入
- [ ] 语义搜索 API（文本查询 → CLIP 向量 → 近邻检索）
- [ ] 视觉相似度 API（图像查询 → 近邻检索）

### 3D 资产处理

- [ ] Blender headless sidecar 集成（`blender -b` 渲染标准格式）
- [ ] 标准格式（OBJ/FBX/GLB/GLTF/STL/USD/PLY）缩略图生成
- [ ] 转台动画帧渲染，帧序列送 AI sidecar 打标
- [ ] Web 交互预览：GLB/GLTF 用 `<model-viewer>`，其他格式用 three.js
- [ ] ZBrush 原生文件（.ztl/.zpr）原地索引，按文件名前缀自动关联导出文件

### 部署更新

- [ ] `docker-compose.yml` 新增 `ai` sidecar 服务和 `blender` sidecar 服务

---

## Phase 2：多用户与远程访问

目标：多用户 RBAC + WebRTC P2P 远程访问 + 移动客户端。

- [ ] 用户注册/登录/会话管理
- [ ] RBAC：角色与权限模型，`owner_id` 字段全面启用
- [ ] 资产/文件夹/标签的多用户隔离
- [ ] WebRTC P2P 远程访问（参考 NasCabOS 方案，无需端口转发）
- [ ] iOS / Android 移动客户端（基础浏览和下载）

---

## Phase 3：生态扩展

目标：MCP 集成、第三方插件市场、云端 AI、转码流媒体。

- [ ] MCP（Model Context Protocol）agent 集成，允许 AI agent 操作 hetu 资产
- [ ] 第三方插件市场：插件发现、安装、版本管理
- [ ] 云端 AI 适配器（OpenAI Vision、Google Vision API 等），vendor-adapter 模式
- [ ] 视频转码与流媒体服务
- [ ] ZBrush 导出触发脚本（ZBrush 保存时自动推送导出文件到 hetu）
- [ ] subprocess + gRPC 插件机制（hashicorp/go-plugin 风格）
- [ ] 独立容器插件机制（docker compose profiles）
- [ ] WASM 插件机制（extism）
