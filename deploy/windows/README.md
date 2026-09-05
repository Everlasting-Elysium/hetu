# Windows 服务器部署

hetu 内核是**纯 Go、无 CGO**(SQLite 用 `modernc.org/sqlite`),因此可在任意平台交叉编译出单个 `.exe`,Windows 上无需 C 工具链。

## 交叉编译

    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o hetu.exe ./cmd/hetu

## 运行

    set HETU_DATA_DIR=C:\hetu\data
    set HETU_LIBRARY_DIR=D:\assets
    set HETU_PLUGINS=dam,nas
    hetu.exe serve

## 注册为 Windows 服务

用 [nssm](https://nssm.cc/) 或 `sc.exe` 将 `hetu.exe serve` 注册为服务。Phase 1 的 AI / Blender / rclone sidecar 建议用 Docker Desktop 或独立进程运行,内核通过环境变量指向其地址。
