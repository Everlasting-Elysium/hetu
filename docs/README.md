# hetu（河图）文档索引

hetu 是一个自托管、AI 原生的平台，通过微内核加插件架构，将 NAS 服务、数字资产管理（DAM）和 AI 能力统一在同一个二进制中。

---

## 文档树

```
docs/
├── README.md          本文件，文档索引与导航
├── overview.md        项目愿景、参考项目对比、目标用户、核心痛点、范围
├── architecture.md    微内核+插件架构、插件机制对比、数据流、目录结构、契约接口
├── tech-stack.md      语言选型、各层库选型、modernc.org/sqlite 理由、sqlite-vec 开放问题
├── ai-and-3d.md       AI 打标流水线、分层元数据规则、3D 可行性结论、本地优先策略
├── data-model.md      数据库表清单与字段说明、owner_id 预留、FTS/向量属于 Phase 1
├── storage.md         存储抽象层、StorageProvider 契约、rclone/AList 路线图
└── roadmap.md         Phase 0/1/2/3 详细任务清单
```

---

## 各文件职责

| 文件 | 职责 |
|------|------|
| [overview.md](./overview.md) | 定义项目是什么、为谁而建、解决什么问题、不做什么。包含与 Billfish/Serpent/NasCabOS 的对比表。 |
| [architecture.md](./architecture.md) | 描述微内核+插件模型的结构与约束、四种插件机制对比与 v0 决策、完整 Go 目录树及每个目录的职责、所有契约接口的概念说明（引用代码路径）。 |
| [tech-stack.md](./tech-stack.md) | 记录语言选型决策（Go vs 其他）、各层依赖库的选择理由、`modernc.org/sqlite` 无 CGO 优势、sqlite-vec 的 CGO 兼容性开放问题。 |
| [ai-and-3d.md](./ai-and-3d.md) | 定义分层元数据规则（manual/ai/extracted，其他文件引用此处）、AI 打标流水线、3D 格式可行性结论表、ZBrush 原生文件处理策略、本地优先原则。 |
| [data-model.md](./data-model.md) | 列出所有数据库表及字段，说明 `owner_id` 预留多用户的设计意图，标注 FTS5 和 embeddings 表属于 Phase 1。 |
| [storage.md](./storage.md) | 说明索引不搬运原则、`StorageProvider` 契约接口、v0 本地文件系统实现、rclone 和 AList/OpenList 的路线图集成方案。 |
| [roadmap.md](./roadmap.md) | 按 Phase 0/1/2/3 列出具体任务清单，每条任务对应一个可交付的工程单元。 |

---

## 阅读顺序建议

首次了解项目：`overview.md` → `architecture.md` → `tech-stack.md`

实现具体功能：`architecture.md`（契约接口）→ `data-model.md`（表结构）→ `storage.md`（存储层）→ `ai-and-3d.md`（AI/3D）

规划工作：`roadmap.md`

---

## 关键设计决策速查

| 决策 | 结论 | 详情 |
|------|------|------|
| 核心语言 | Go，模块路径 `github.com/Everlasting-Elysium/hetu` | [tech-stack.md](./tech-stack.md) |
| 插件机制（v0） | 配置期编译模块，`HETU_PLUGINS` 环境变量控制 | [architecture.md](./architecture.md) |
| 数据库 | SQLite，`modernc.org/sqlite`（无 CGO） | [tech-stack.md](./tech-stack.md) |
| 向量搜索 | sqlite-vec，CGO 兼容性为开放问题 | [tech-stack.md](./tech-stack.md) |
| 分层元数据 | manual > ai > extracted，AI 层可单独清除重跑 | [ai-and-3d.md](./ai-and-3d.md) |
| 存储抽象 | StorageProvider 接口，v0 仅本地 FS | [storage.md](./storage.md) |
| ZBrush 文件 | 托管不透明资产，关联用户导出文件 | [ai-and-3d.md](./ai-and-3d.md) |
| 多用户 | owner_id 从第一天预留，Phase 2 叠加 | [data-model.md](./data-model.md) |
