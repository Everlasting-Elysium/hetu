# AI 与 3D 处理

## AI Sidecar 架构

AI 推理运行在独立的 Python 进程或容器中（`ai/` 目录），通过 HTTP 或 gRPC 与 Go 内核通信。Go 内核不直接加载任何 AI 模型。

这种设计的理由：
- Python 的 AI 生态（PyTorch、transformers、CLIP、ONNX Runtime）远优于 Go
- AI sidecar 可独立升级、替换模型，不影响内核
- 本地模型是默认选择，保护隐私、无推理费用；云端 AI 适配器作为 Phase 3 可选项（参考 Serpent 的 vendor-adapter 模式）

AI sidecar 提供的能力：

| 能力 | 说明 |
|------|------|
| 图像打标（tagger） | 输出标签列表及置信度 |
| 图像描述（captioner） | 输出自然语言描述 |
| CLIP 嵌入 | 输出图像或文本的向量表示，用于语义搜索和视觉相似度 |
| 人脸检测 | 检测并聚类人脸 |
| OCR | 从图像/文档中提取文字 |
| 图像对比点评（VLM，可插拔） | 一次推理同时看两张图，输出自然语言点评；经 `kernel.VisionCritic`（nil=不可用）接入,未配置 `HETU_AI_VLM_MODEL` 时降级为「点评不可用」,不影响其余确定性对比维度(issue #127) |

---

## 分层元数据规则

hetu 的元数据分三层，借鉴 Serpent 的 vendor-adapter 模式并扩展：

| 层 | 名称 | 来源 | 优先级 | 可清除/重跑 |
|----|------|------|--------|-------------|
| `manual` | 手动层 | 用户直接编辑 | 最高，永远不被覆盖 | 否（用户主动删除） |
| `ai` | AI 层 | AI sidecar 推理结果 | 中，不覆盖 manual | 是，可单独清除并重跑 |
| `extracted` | 提取层 | 文件内嵌元数据（EXIF、ID3、XMP 等） | 最低 | 随重新索引自动更新 |

**核心规则**：
- `manual` 层的值永远不被 AI 或提取流程覆盖
- `ai` 层可在模型升级后整体清除并重新运行，不影响用户手动标注
- 同一字段在多层都有值时，按优先级取最高层的值展示

数据模型中的对应表为 `annotations`，字段 `layer` 存储层标识。详见 [data-model.md](./data-model.md)。

---

## AI 自动打标流水线（Phase 1）

```
资产导入（索引完成事件）
  → JobQueue 入队 AI 打标任务
    → Go 内核调用 AI sidecar HTTP/gRPC 接口
      → sidecar 解码图像（或接收客户端截图）
        → tagger：输出标签 + 置信度
        → captioner：输出描述文本
        → CLIP encoder：输出向量嵌入
      → Go 内核将结果写入 annotations 表（layer=ai）
      → 写入 FTS5 索引（全文检索）
      → 写入 embeddings 表（向量检索，sqlite-vec，见 tech-stack.md 开放问题）
```

打标结果写入 `ai` 层后，不触碰 `manual` 层已有的任何值。

---

## 3D 资产处理

### 可行性结论表

| 格式类型 | 具体格式 | 缩略图/预览 | Web 交互预览 | AI 打标 | 说明 |
|----------|----------|-------------|--------------|---------|------|
| 标准交换格式 | OBJ, FBX, GLB, GLTF, STL, USD, PLY | 客户端截图（`<model-viewer>` toBlob 回传） | `<model-viewer>`（GLB/GLTF 直出，其他格式经 assimp 转 GLB） | 客户端截图送 AI sidecar | 完全支持 |
| ZBrush 原生 | .ztl, .zpr | 不可靠（见下） | 不支持 | 间接支持 | 专有闭源二进制，见下节 |

### 缩略图策略（issue #78）

3D 模型缩略图由**客户端截图**唯一提供，服务端不参与渲染：

用户首次在 `<model-viewer>` 打开模型时，组件自动调用 `toBlob()` 截取当前视图，回传 `POST /api/dam/assets/{id}/thumb` 写入 `assets.thumb_path`。仅在缺缩略时执行一次，后续请求直接返回已缓存的缩略图。服务端的 `model3d.Handler.Thumbnail()` 始终返回 `domain.ErrNoThumbnail`，索引期间 3D 资产以无预览状态入库，客户端首次查看时自动补全。

**优雅降级**：用户未打开过该模型时，缩略图为空，资产照常入库，扫描不中断。

实现见 [thumb_upload.go](../internal/plugins/dam/thumb_upload.go)（后端）和 [ModelViewer.tsx](../web/src/components/ModelViewer.tsx)（前端）。

### 格式转换策略（issue #78）

非 web 原生格式（OBJ/FBX/STL/USD/PLY）需转为 GLB 供 `<model-viewer>` 预览。转换后端通过 `HETU_MODEL_CONVERTER` 环境变量选择：

| 后端 | 配置值 | 依赖 | 适用场景 |
|------|--------|------|----------|
| assimp | `assimp` | `assimp` CLI 在 PATH | 轻量自托管，纯 CPU，无 GPU/容器要求 |
| 自动探测 | 空（默认） | 探测 assimp 是否在 PATH | 开箱即用；assimp 不可用则 `/model` 端点对非 web 原生格式返回 503 |

- assimp 以子进程方式调用（`assimp export <in> <out.glb>`），无需 HTTP sidecar，适合简单无状态转换（参考：[架构决策] 简单无状态格式转换工具用子进程调用）。
- 转换结果按内容哈希缓存（`ModelCacheDir`），同一文件仅转换一次。
- 并发转换通过 singleflight + 信号量限流，避免资源耗尽。

接口定义：[kernel.ModelConverter](../internal/kernel/kernel.go)。工厂函数：[model3d.NewConverter](../internal/asset/model3d/converter.go)。

### 标准格式处理流程

Web 交互预览：
- GLB/GLTF 格式：`<model-viewer>` 直接加载，零转换
- 其他格式：经 assimp 转为 GLB，再由 `<model-viewer>` 渲染

### 3D 渲染即打标的洞察

标准 3D 格式的 AI 打标不需要专门的 3D 理解模型。流程为：

```
客户端截图（<model-viewer> toBlob 回传）
  → 图像送 AI sidecar（tagger + CLIP）
  → 产出：预览图 + 标签 + 向量嵌入
```

一条流水线同时完成预览生成和 AI 打标，无需额外步骤。

### ZBrush 原生格式（.ztl / .zpr）

**现实约束**：
- ZBrush 原生格式是专有闭源二进制，无公开规范
- 文件通常超过 2GB（5000 万以上多边形），无法在 ZBrush 外解析或渲染
- 文件内嵌有缩略图，但提取依赖逆向工程，结果不可靠

**hetu 的策略**：将 ZBrush 原生文件作为**托管不透明资产**处理：

1. 文件原地索引，记录路径、大小、哈希、修改时间等基础元数据
2. 自动关联同名的用户导出文件（如 `sculpture.ztl` 关联 `sculpture.obj` 或 `sculpture_preview.png`），匹配规则基于文件名前缀
3. 关联的标准格式文件走正常的 3D 渲染打标流水线
4. 用户可在 hetu 中手动添加标签、描述、评分（写入 `manual` 层）
5. 可选的未来扩展：ZBrush 导出时触发脚本，自动将导出文件推送给 hetu（Phase 3 路线图）

---

## 3D 格式转换部署

### assimp

安装 `assimp` CLI 到 hetu 镜像或宿主机 PATH 即可，无需额外容器：

```bash
# Debian/Ubuntu
apt-get install -y assimp-utils
# macOS
brew install assimp
```

hetu 启动时自动探测 `assimp` 可用性。也可显式指定：`HETU_MODEL_CONVERTER=assimp`。配置项定义见 [config.go](../internal/config/config.go)。

### 优雅降级

3D 处理全链路尽力而为:
- **缩略图**：用户未打开模型时 `ThumbPath` 为空，扫描不中断；用户首次查看时客户端自动补缩略。
- **格式转换**：assimp 不可用时，非 web 原生格式返回 503，GLB/GLTF 不受影响。
- **配置检测**：`HETU_MODEL_CONVERTER` 为空时自动探测 assimp；不可用则 `ModelConverter` 为 nil。

---

## 本地优先，云端可选

默认所有 AI 推理在本地运行，原因：
- 用户资产（尤其是未发布的 3D 作品）不应上传到第三方服务
- 无推理费用，适合批量处理大量资产
- 网络不可用时仍可正常工作

云端 AI 适配器（如 OpenAI Vision、Google Vision API）作为 Phase 3 可选项，通过与 Serpent 类似的 vendor-adapter 模式接入，不改变内核和 sidecar 接口。
