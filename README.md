# hetu(河图)

自托管、AI 原生的 **NAS + 资源管理(DAM)** 平台。采用**微内核 + 插件**架构:内核是平台,**DAM 和 NAS 是两个能力插件**。

> 本仓库为 hetu 的**公开发布门面**,仅提供下载与发行说明;源码为专有闭源。

## 下载

前往 [Releases](https://github.com/Everlasting-Elysium/hetu/releases/latest) 下载开箱即用的安装包(内置 ffmpeg / poppler / LibreOffice,视频 / PDF / office 缩略图无需额外安装):

| 平台 | 安装包 | 安装说明 |
|---|---|---|
| **Windows** | `hetu-setup-x64.exe` | [deploy/windows](https://github.com/Everlasting-Elysium/hetu/releases/latest) |
| **macOS**(Apple Silicon / Intel 通用) | `hetu.dmg` | 下载后拖入「应用程序」 |
| **Docker**(服务器 / NAS) | `ghcr.io/everlasting-elysium/hetu:latest` | 见下方命令 |

    docker run -p 8080:8080 -v hetu-data:/data -v /path/to/assets:/library \
      ghcr.io/everlasting-elysium/hetu:latest

安装后可用 `hetu update` 自动检查并升级到最新版本。

## 快速开始

    # 索引一个素材目录(索引不搬运原文件)
    HETU_LIBRARY_DIR=/path/to/assets hetu scan

    # 启动服务
    HETU_LIBRARY_DIR=/path/to/assets hetu serve
    # GET http://localhost:8080/healthz
    # GET http://localhost:8080/api/dam/assets
    # GET http://localhost:8080/api/nas/browse?path=

## 配置

全部通过环境变量。关键项:`HETU_PLUGINS`(默认 `dam,nas`)、`HETU_LIBRARY_DIR`、`HETU_DATA_DIR`、`HETU_DB_PATH`、`HETU_ADDR`。

## 许可

hetu 本体为专有软件,二进制发行版供个人非商业使用。发行版捆绑的第三方组件(ffmpeg / poppler / LibreOffice)遵循各自许可,见 [THIRD-PARTY-LICENSES.md](THIRD-PARTY-LICENSES.md)。
