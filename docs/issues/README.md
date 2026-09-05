# hetu 实现 Issue 看板(总体设计)

本文件是 GitHub Issue 跟踪器的**仓库内索引 + 协作规范**。13 个实现 issue 已建在 tracker(见下表),Epic 汇总见 [#13](https://github.com/Everlasting-Elysium/hetu/issues/13)。整体设计由架构负责人维护;实现者认领单个 issue 即可。每个 issue 的正文都回链本文件的「全局约束」。

---

## 全局约束(每个实现者必须遵守,违反视为未完成)

这些约束来自 [../architecture.md](../architecture.md) 与 `AGENTS.md`,是保证多 agent 并行不产生冲突/劣化的前提:

1. **尊重内核契约**:插件实现 `kernel.Plugin`;存储实现 `kernel.StorageProvider`;资产类型实现 `kernel.AssetHandler`;持久化经 `kernel.Store` 接口。不要绕过内核直接耦合。
2. **索引不搬运**:绝不复制/移动用户原文件,只在库中登记 `storage_path`(见 [../storage.md](../storage.md))。
3. **分层元数据非破坏**:`manual > ai > extracted`;**AI 层永不覆盖 manual 层**,可单独清除重跑(见 [../ai-and-3d.md](../ai-and-3d.md))。
4. **DB 用 sqlc 生成**:改 `internal/store/schema.sql` + `internal/store/queries/*.sql` 后跑 `sqlc generate`;`internal/store/db/` 不手写。
5. **无 CGO**:保持 `modernc.org/sqlite`;引入 CGO(如 sqlite-vec)必须先在 [#10](https://github.com/Everlasting-Elysium/hetu/issues/10) 的开放问题里论证并取得架构负责人确认。
6. **工程规范**:`slog` 记录日志(禁 `log.*`/`fmt.Println`);I/O 函数 `context.Context` 首参;错误 `%w` 包裹 + 类型化 sentinel;parse-don't-validate(smart constructor);**单文件 ≤250 纯 LOC**;每次交付 `go build ./... && go vet ./... && go test -race ./...` 全绿。
7. **契约变更需协调**:任何对 `internal/kernel` 契约接口的修改,先在 [#1](https://github.com/Everlasting-Elysium/hetu/issues/1) 或对应 issue 评论里同步,避免破坏其他并行分支。

---

## 依赖图与建议并行批次

```
批次 A(可立即并行):
  #1 数据模型扩展  ──┬─> #2 DAM 文件夹/标签
                    ├─> #3 全文检索(FTS5)
                    └─> #4 NAS 下载/分享链接
  #5 视频/文档处理器(独立)
  #7 rclone/AList 存储 provider(独立)
  #8 AI sidecar 契约+客户端(独立)

批次 B(依赖 A):
  #6  缩略图服务 + Web UI       (依赖 #2/#3 的 API)
  #9  AI 自动打标流水线         (依赖 #8 + #1)
  #11 Python AI 模型实现        (依赖 #8 契约)
  #12 3D 标准格式 + 预览         (依赖 #5 的处理器模式)

批次 C(依赖 B):
  #10 CLIP 嵌入 + 语义/相似搜索  (依赖 #9/#11 + sqlite-vec 决策)
  #15 3D 渲染打标 + ZBrush 关联   (依赖 #12 + #9)
```

---

## Issue 一览

| # | 标题 | Phase | 建议 category / skills | 依赖 |
|---|------|-------|------------------------|------|
| [#1](https://github.com/Everlasting-Elysium/hetu/issues/1) | 数据模型扩展(folders/tags/annotations/shares/jobs) | 0 | unspecified-high / [programming] | — |
| [#2](https://github.com/Everlasting-Elysium/hetu/issues/2) | DAM:文件夹 + 层级标签 + 打标/评分/颜色 | 0 | unspecified-high / [programming] | #1 |
| [#3](https://github.com/Everlasting-Elysium/hetu/issues/3) | 全文检索(FTS5)+ 搜索 API | 0/1 | ultrabrain / [programming] | #1 |
| [#4](https://github.com/Everlasting-Elysium/hetu/issues/4) | NAS:文件下载 + 分享链接 | 0 | unspecified-high / [programming] | #1 |
| [#5](https://github.com/Everlasting-Elysium/hetu/issues/5) | 资产处理器:视频(ffmpeg)+ 文档/PDF | 0/1 | deep / [programming] | — |
| [#6](https://github.com/Everlasting-Elysium/hetu/issues/6) | 缩略图服务 + 最小 Web UI | 0 | visual-engineering / [frontend, playwright] | #2,#3 |
| [#7](https://github.com/Everlasting-Elysium/hetu/issues/7) | 存储 provider:rclone/AList(网盘当 S3) | 0/1 | deep / [programming] | — |
| [#8](https://github.com/Everlasting-Elysium/hetu/issues/8) | AI sidecar 契约 + Go 客户端 + 任务编排 | 1 | deep / [programming] | — |
| [#9](https://github.com/Everlasting-Elysium/hetu/issues/9) | AI 自动打标流水线(解决痛点①) | 1 | deep / [programming] | #8,#1 |
| [#10](https://github.com/Everlasting-Elysium/hetu/issues/10) | CLIP 嵌入 + 语义/视觉相似搜索 | 1 | ultrabrain / [programming] | #9,#11 |
| [#11](https://github.com/Everlasting-Elysium/hetu/issues/11) | Python AI sidecar 模型实现 | 1 | deep / [programming] | #8 |
| [#12](https://github.com/Everlasting-Elysium/hetu/issues/12) | 3D 标准格式处理器 + Blender 缩略 + 预览(痛点②) | 1 | deep / [programming, frontend] | #5 |
| [#15](https://github.com/Everlasting-Elysium/hetu/issues/15) | 3D 渲染打标 + ZBrush 资产关联 | 1 | deep / [programming] | #12,#9 |
| [#13](https://github.com/Everlasting-Elysium/hetu/issues/13) | **[Epic]** 整体设计与实现路线(总览) | — | epic | — |

> 认领建议:每个 agent 用对应 `category` + `load_skills` 起 `task(...)`;批次 A 的 #1/#5/#7/#8 可立即并行开工。

---

## 下一波(Phase 2/3,待细化)

Phase 2:多用户/RBAC、WebRTC P2P 远程访问、移动端。Phase 3:MCP agent 集成、第三方插件市场、云端 AI 适配器、视频转码流媒体、子进程/容器/WASM 插件机制。细化后在 tracker 追加 issue 并更新本表。清单见 [../roadmap.md](../roadmap.md)。
