# 第三方组件许可（THIRD-PARTY-LICENSES）

hetu 本体为专有软件（见 [LICENSE](LICENSE)）。但**二进制发行版**（Windows `.exe`、macOS `.dmg`、Docker 镜像）为了让视频 / PDF / office 缩略图开箱即用，**捆绑重分发**了以下第三方命令行工具。hetu 以**独立子进程方式调用**它们（非链接、非派生作品，属 GPL 意义上的“聚合”），因此 hetu 自身源码可保持闭源；但被重分发的这些工具**各自保留其原始许可**，且我们负有随发行版提供其许可证文本、以及（对 GPL 组件）提供对应源码或书面获取途径的义务。

| 组件 | 用途 | 许可 | 分发中的来源 |
|---|---|---|---|
| **FFmpeg** | 视频缩略图 / 探测 | **GPL**（Windows 使用 `*-win64-gpl` 构建；macOS 使用 martin-riedl 静态构建） | 二进制随安装包捆绑 |
| **Poppler** | PDF 缩略图 | **GPL v2 / v3** | Windows: oschwartz10612/poppler-windows；macOS: Homebrew |
| **LibreOffice** | office 文档缩略图 | **MPL 2.0**（部分组件 LGPL v3） | 官方 Still 分支离线安装包 |

## GPL 合规义务（FFmpeg / Poppler）

重分发 GPL 二进制时须：

1. **随附许可证全文**——将各工具的 `COPYING` / `LICENSE` 文本一并打进安装包与 Docker 镜像。
2. **提供对应源码**——满足以下任一：
   - 随附完整对应源码；或
   - 附一份**书面 offer**，声明可按 GPL 条款索取对应源码（须保留三年）。
   实践上：由于这些是**未经修改**的上游构建，可通过链接到我们所用的确切上游发布版本（含版本号 / commit / 构建产物 URL）来满足。各版本号见每次 Release 的 `manifest.json`。

## LibreOffice（MPL 2.0）

MPL 2.0 允许与专有软件一起分发，须保留其许可与版权声明。未修改上游二进制时，随附其许可文本即可。

---

> TODO（打包跟进）：把上述各工具的许可证全文 + 上游源码链接实际打进
> `deploy/windows/installer.iss` 与 `deploy/macos/build-app.sh` 的产物，以及
> Docker 镜像。当前文件先记录义务，尚未在安装包内落地。
