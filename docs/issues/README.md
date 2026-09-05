# hetu 实现 Issue 看板(总体设计)

**当前优先级:先交付完整的资产管理(DAM)工具** —— 里程碑 [DAM v1](https://github.com/Everlasting-Elysium/hetu/milestone/1),`priority:high` 的 20 个 issue。**NAS / 网盘 / 远程访问往后推**(`priority:later`)。Epic 汇总见 [#13](https://github.com/Everlasting-Elysium/hetu/issues/13)。每个 issue 正文都回链本文件的「全局约束」。

- **DAM v1(优先)**:标签 `priority:high` + 里程碑 `DAM v1`。
- **往后推**:标签 `priority:later` —— [#7](https://github.com/Everlasting-Elysium/hetu/issues/7) 网盘存储、[#24](https://github.com/Everlasting-Elysium/hetu/issues/24) 多资源库,及 Phase 2/3(见文末)。

---

## 全局约束(每个实现者必须遵守,违反视为未完成)

这些约束来自 [../architecture.md](../architecture.md) 与 `AGENTS.md`,是保证多 agent 并行不产生冲突/劣化的前提:

1. **尊重内核契约**:插件实现 `kernel.Plugin`;存储实现 `kernel.StorageProvider`;资产类型实现 `kernel.AssetHandler`;持久化经 `kernel.Store` 接口。不要绕过内核直接耦合。
2. **索引不搬运**:`index` 模式绝不复制/移动用户原文件,只登记 `storage_path`(见 [../storage.md](../storage.md);`copy`/`move` 导入是显式例外,见 [#18](https://github.com/Everlasting-Elysium/hetu/issues/18))。
3. **分层元数据非破坏**:`manual > ai > extracted`;**AI 层永不覆盖 manual 层**,可单独清除重跑(见 [../ai-and-3d.md](../ai-and-3d.md))。
4. **DB 用 sqlc 生成**:改 `internal/store/schema.sql` + `internal/store/queries/*.sql` 后跑 `sqlc generate`;`internal/store/db/` 不手写。
5. **无 CGO**:保持 `modernc.org/sqlite`;引入 CGO(如 sqlite-vec)必须先在 [#10](https://github.com/Everlasting-Elysium/hetu/issues/10) 的开放问题里论证并取得架构负责人确认。
6. **工程规范**:`slog` 记录日志(禁 `log.*`/`fmt.Println`);I/O 函数 `context.Context` 首参;错误 `%w` 包裹 + 类型化 sentinel;parse-don't-validate(smart constructor);**单文件 ≤250 纯 LOC**;每次交付 `go build ./... && go vet ./... && go test -race ./...` 全绿。
7. **契约变更需协调**:任何对 `internal/kernel` 契约接口的修改,先在 [#1](https://github.com/Everlasting-Elysium/hetu/issues/1) 或对应 issue 评论里同步,避免破坏其他并行分支。

---

## DAM v1 依赖批次(建议并行)

```
批次 A(无依赖,立即并行):
  #1 数据模型(根依赖)    #5 视频/文档处理器    #8 AI sidecar 契约+客户端

批次 B(依赖 #1,互相可并行):
  #2 文件夹/标签/评分/颜色   #3 全文检索 FTS5     #4 下载/分享(资产导出)
  #16 颜色提取+颜色搜索      #18 导入模式+API     #19 文件监听+自动重索引
  #20 EXIF/IPTC/XMP 提取    #22 重复检测

批次 C:
  #6  缩略图+最小 UI     (依赖 #2,#3)     #17 智能文件夹     (依赖 #2,#3)
  #21 批量操作+回收站    (依赖 #2)         #9  AI 自动打标    (依赖 #8,#1)
  #11 Python AI 模型     (依赖 #8)         #12 3D 标准格式+预览 (依赖 #5)

批次 D:
  #10 CLIP 语义/相似搜索 (依赖 #9,#11)     #15 3D 渲染打标+ZBrush (依赖 #12,#9)
  #23 DAM Web UI 完整交互 (依赖 #6 + 各功能 API)
```

---

## Issue 一览(DAM v1,priority:high)

| # | 标题 | 建议 category / skills | 依赖 |
|---|------|------------------------|------|
| [#1](https://github.com/Everlasting-Elysium/hetu/issues/1) | 数据模型扩展(folders/tags/annotations/shares/jobs) | unspecified-high / [programming] | — |
| [#5](https://github.com/Everlasting-Elysium/hetu/issues/5) | 资产处理器:视频(ffmpeg)+ 文档/PDF | deep / [programming] | — |
| [#8](https://github.com/Everlasting-Elysium/hetu/issues/8) | AI sidecar 契约 + Go 客户端 + 任务编排 | deep / [programming] | — |
| [#2](https://github.com/Everlasting-Elysium/hetu/issues/2) | 文件夹 + 层级标签 + 打标/评分/颜色 | unspecified-high / [programming] | #1 |
| [#3](https://github.com/Everlasting-Elysium/hetu/issues/3) | 全文检索(FTS5)+ 搜索 API | ultrabrain / [programming] | #1 |
| [#4](https://github.com/Everlasting-Elysium/hetu/issues/4) | 资产下载 + 分享链接 | unspecified-high / [programming] | #1 |
| [#16](https://github.com/Everlasting-Elysium/hetu/issues/16) | 颜色提取(调色板)+ 颜色搜索 | deep / [programming] | #1 |
| [#18](https://github.com/Everlasting-Elysium/hetu/issues/18) | 导入模式(复制/移动/索引)+ 导入 API | deep / [programming] | #1 |
| [#19](https://github.com/Everlasting-Elysium/hetu/issues/19) | 文件监听 + 自动重新索引 | deep / [programming] | #1 |
| [#20](https://github.com/Everlasting-Elysium/hetu/issues/20) | EXIF/IPTC/XMP 元数据提取(extracted 层) | deep / [programming] | #1 |
| [#22](https://github.com/Everlasting-Elysium/hetu/issues/22) | 重复检测(内容 hash + 可选 pHash) | deep / [programming] | #1 |
| [#6](https://github.com/Everlasting-Elysium/hetu/issues/6) | 缩略图服务 + 最小 Web UI | visual-engineering / [frontend, playwright] | #2,#3 |
| [#17](https://github.com/Everlasting-Elysium/hetu/issues/17) | 智能文件夹 / 保存的搜索 | unspecified-high / [programming] | #1,#2,#3 |
| [#21](https://github.com/Everlasting-Elysium/hetu/issues/21) | 批量操作:重命名/打标/评分/移动/回收站+恢复 | deep / [programming] | #1,#2 |
| [#9](https://github.com/Everlasting-Elysium/hetu/issues/9) | AI 自动打标流水线(痛点①) | deep / [programming] | #8,#1 |
| [#11](https://github.com/Everlasting-Elysium/hetu/issues/11) | Python AI sidecar 模型实现 | deep / [programming] | #8 |
| [#12](https://github.com/Everlasting-Elysium/hetu/issues/12) | 3D 标准格式处理器 + Blender 缩略 + 预览(痛点②) | deep / [programming, frontend] | #5 |
| [#10](https://github.com/Everlasting-Elysium/hetu/issues/10) | CLIP 嵌入 + 语义/视觉相似搜索 | ultrabrain / [programming] | #9,#11 |
| [#15](https://github.com/Everlasting-Elysium/hetu/issues/15) | 3D 渲染打标 + ZBrush 资产关联 | deep / [programming] | #12,#9 |
| [#23](https://github.com/Everlasting-Elysium/hetu/issues/23) | DAM Web UI 完整交互(详情/标签/智能夹/批量/颜色搜索) | visual-engineering / [frontend, playwright] | #6 + 各 API |

> 认领:每个 agent 用对应 `category` + `load_skills` 起 `task(...)`;批次 A 的 #1/#5/#8 现在就能并行开工。

---

## 往后推(priority:later)

- [#7](https://github.com/Everlasting-Elysium/hetu/issues/7) 存储 provider:rclone/AList(网盘当 S3)—— NAS 能力,DAM v1 之后再做
- [#24](https://github.com/Everlasting-Elysium/hetu/issues/24) 多资源库支持
- **Phase 2**:多用户/RBAC、WebRTC P2P 远程访问、移动端
- **Phase 3**:MCP agent 集成、第三方插件市场、云端 AI 适配器、视频转码流媒体、子进程/容器/WASM 插件机制

清单见 [../roadmap.md](../roadmap.md)。细化后在 tracker 追加 issue 并更新本表。
