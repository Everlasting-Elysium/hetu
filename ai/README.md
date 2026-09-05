# hetu AI sidecar

本地优先的 AI 能力(Phase 1 实现),作为独立进程/容器运行,Go 内核通过 HTTP 调用。

- `server.py` — FastAPI 服务;当前为**契约桩**(`/health` 可用,`/embed`、`/tag` 返回 501)。
- Phase 1 将实现:CLIP 语义 / 以图搜图嵌入、图像自动打标、caption、人脸、OCR;3D 模型先用 Blender 渲染转盘,再走同一条打标流水线。
- 云端 AI(可选)按 Serpent 的 vendor-adapter 模式接入。

设计见 [../docs/ai-and-3d.md](../docs/ai-and-3d.md)。

## 开发启动

    pip install -r requirements.txt
    uvicorn server:app --port 8091
