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
      → sidecar 解码图像（或接收 Blender 渲染帧）
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
| 标准交换格式 | OBJ, FBX, GLB, GLTF, STL, USD, PLY | Blender headless 渲染 | three.js（所有格式）/ `<model-viewer>`（仅 GLB/GLTF） | 渲染帧送 AI sidecar | 完全支持 |
| ZBrush 原生 | .ztl, .zpr | 不可靠（见下） | 不支持 | 间接支持 | 专有闭源二进制，见下节 |

### 标准格式处理流程

Blender headless（`blender -b`）作为 sidecar 运行，接受标准 3D 格式文件，输出：
1. 静态缩略图（PNG）
2. 转台动画帧序列（用于 AI 打标）

Web 交互预览：
- GLB/GLTF 格式：优先使用 `<model-viewer>`（Google，声明式，零配置）
- 其他格式：使用 three.js 加载对应 loader，或先转换为 GLB 再用 `<model-viewer>`

### 3D 渲染即打标的洞察

标准 3D 格式的 AI 打标不需要专门的 3D 理解模型。流程为：

```
Blender headless 渲染转台帧
  → 多帧图像送 AI sidecar（tagger + CLIP）
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

## Blender Sidecar 部署

标准 3D 格式的缩略图由 Blender headless sidecar 渲染，与内核解耦：内核通过 HTTP 调用 sidecar，sidecar 内部运行 `blender -b -P render.py`。3D 资产处理器实现见 [internal/asset/model3d](../internal/asset/model3d)，支持格式:OBJ、FBX、GLB、GLTF、STL、USD、PLY。

### 启动

Blender sidecar 属于 `media` compose profile，默认不启动:

```
docker compose --profile media up
```

启动后需在 hetu 服务上设置 `HETU_BLENDER_ADDR`(容器内指向 `blender:9090`,见 [docker-compose.yml](../deploy/docker-compose.yml) 中默认注释的一行),3D 缩略即开启。配置项定义见 [config.go](../internal/config/config.go) 的 `BlenderAddr` 字段。

### 渲染脚本

| 脚本 | 职责 |
|------|------|
| [render.py](../deploy/blender/render.py) | Blender 无头渲染:导入模型、按包围盒自动取景相机、三点布光、EEVEE 引擎渲染 512×512 透明背景 PNG。用法 `blender -b -P render.py -- <输入> <输出>`。 |
| [server.py](../deploy/blender/server.py) | Flask HTTP 包装:`POST /render` 接收 multipart 模型文件,按魔数嗅探格式后调用 render.py,返回 PNG。监听地址由 `BLENDER_LISTEN`(默认 `:9090`)控制。 |

> `linuxserver/blender` 镜像需具备 `python3` 与 `flask`;若缺失,在镜像内 `pip install flask` 或改用自建镜像。

### 优雅降级

3D 缩略是尽力而为的:当 `HETU_BLENDER_ADDR` 为空、sidecar 不可达或渲染失败时,模型资产照常入库(记录路径、大小、哈希、类型 `model`),仅 `ThumbPath` 为空——扫描永不因 3D 缩略失败而中断。

---

## 本地优先，云端可选

默认所有 AI 推理在本地运行，原因：
- 用户资产（尤其是未发布的 3D 作品）不应上传到第三方服务
- 无推理费用，适合批量处理大量资产
- 网络不可用时仍可正常工作

云端 AI 适配器（如 OpenAI Vision、Google Vision API）作为 Phase 3 可选项，通过与 Serpent 类似的 vendor-adapter 模式接入，不改变内核和 sidecar 接口。
