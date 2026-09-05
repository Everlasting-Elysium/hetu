# hetu AI sidecar

本地优先的 AI 能力,作为独立进程/容器运行,Go 内核通过 HTTP 调用。所有推理默认在
**本地 CPU** 上运行,不产生云端调用;模型权重下载后缓存到磁盘,首次调用时按需加载。

HTTP 契约由 Go 客户端 [internal/ai/types.go](../internal/ai/types.go) 定义(契约版本
`v1`),本服务的请求/响应 JSON 必须与之逐字段一致。

## 端点

| 方法 | 路径 | 请求体 | 响应(见 `schemas.py`) | 模型 |
|------|------|--------|------------------------|------|
| GET  | `/health`  | —                 | `{"ok": true}` | 无(进程就绪即可) |
| POST | `/embed`   | `{"ref": "..."}`  | `{"vector": [...], "dim": N, "model": ""}` | CLIP |
| POST | `/tag`     | `{"ref": "..."}`  | `{"tags": [{"name","confidence"}], "caption": "", "model": ""}` | WD tagger |
| POST | `/caption` | `{"ref": "..."}`  | `{"caption": "", "model": ""}` | BLIP |
| POST | `/ocr`     | `{"ref": "..."}`  | `{"text": "", "blocks": [{"text","confidence","bbox"}], "model": ""}` | RapidOCR |

`ref` 是本地存储路径或 `http(s)` URL,由 sidecar 本地解析为图像字节;解析失败返回
`400`(Go 侧映射为终态 `KindInvalid`,不重试)。`/embed` 特殊:当 `ref` 无法解析为图像
时,退化为把 `ref` 当作 CLIP **文本**查询编码,用于语义检索。

## 模型来源与体积

模型从 Hugging Face 下载并缓存到 `HETU_AI_CACHE_DIR`(默认 `~/.cache/hetu-ai`,容器内
`/models`)。均可通过环境变量替换。

| 能力 | 默认模型(可配置) | 运行时 | 体积(约) | 维度 |
|------|--------------------|--------|-----------|------|
| CLIP 嵌入 | [`openai/clip-vit-base-patch32`](https://huggingface.co/openai/clip-vit-base-patch32) | transformers + torch | ~605 MB | 512 |
| 图像打标 | [`SmilingWolf/wd-v1-4-moat-tagger-v2`](https://huggingface.co/SmilingWolf/wd-v1-4-moat-tagger-v2) | onnxruntime | ~326 MB | — |
| 图像描述 | [`Salesforce/blip-image-captioning-base`](https://huggingface.co/Salesforce/blip-image-captioning-base) | transformers + torch | ~990 MB | — |
| OCR | RapidOCR 内置 PP-OCRv4(det+rec ONNX) | onnxruntime | ~15 MB(随 wheel 分发,无需下载) | — |

## 配置(环境变量,前缀 `HETU_AI_`,定义见 [config.py](config.py))

| 变量 | 默认 | 说明 |
|------|------|------|
| `HETU_AI_CACHE_DIR` | `~/.cache/hetu-ai` | 模型磁盘缓存目录(映射到 `HF_HOME`) |
| `HETU_AI_DEVICE` | `cpu` | `cpu` 或 `cuda`(GPU 需 CUDA 版 torch/onnxruntime 与相应基础镜像) |
| `HETU_AI_CLIP_MODEL` / `HETU_AI_TAGGER_REPO` / `HETU_AI_CAPTION_MODEL` | 见上表 | 覆盖默认模型 |
| `HETU_AI_TAG_THRESHOLD` / `HETU_AI_TAG_CHAR_THRESHOLD` | `0.35` / `0.75` | 一般/角色标签置信度阈值 |
| `HETU_AI_MAX_TAGS` | `30` | 单张图返回标签数上限 |
| `HETU_AI_MAX_CONCURRENCY` | `2` | 并发推理上限(超出排队,防止内存打满) |
| `HETU_AI_CAPTION_MAX_TOKENS` | `40` | caption 生成的最大新 token 数 |

## 架构

- [server.py](server.py) — FastAPI 应用与路由;推理经 [runtime.py](runtime.py) 的容量限制器
  在工作线程中执行,不阻塞事件循环。
- [schemas.py](schemas.py) — 与 Go 契约逐字段对应的 pydantic v2 模型(**唯一事实来源**)。
- [resolver.py](resolver.py) — 把 `ref` 解析为 `PIL.Image` 或文本查询。
- [embed.py](embed.py) / [tagger.py](tagger.py) / [caption.py](caption.py) / [ocr.py](ocr.py)
  — 各能力的模型加载与推理;模型经 `runtime.Lazy` 单次惰性加载并缓存。

设计见 [../docs/ai-and-3d.md](../docs/ai-and-3d.md)。

## 开发

依赖与工具链用 [uv](https://docs.astral.sh/uv/)(torch 走 CPU wheel 源,见 `pyproject.toml`)。

    uv sync --all-groups            # 安装依赖 + 开发工具
    uv run ruff check .             # lint
    uv run basedpyright             # 类型检查
    uv run pytest                   # 契约 + 单元测试(不下载权重)
    uv run uvicorn server:app --port 8091   # 本地启动

`requirements.txt` 由 `uv export` 生成,供不使用 uv 的 pip 用户回退。

## 容器

    docker build -t hetu-ai:dev ./ai
    docker run --rm -p 8091:8091 -v hetu-ai-models:/models hetu-ai:dev

或经 compose(profile `ai`,见 [../deploy/docker-compose.yml](../deploy/docker-compose.yml)):

    docker compose --profile ai up

挂载 `/models` 卷可让模型只下载一次。云端 AI(可选)按 Serpent 的 vendor-adapter 模式
在 Phase 3 接入,不改变内核与 sidecar 接口。
