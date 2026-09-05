# AGENTS.md

Go 1.26+ self-hosted, AI-native NAS + digital-asset-management platform.
Microkernel + plugins: the kernel is a platform; DAM and NAS are capability
plugins enabled via `HETU_PLUGINS`. Design docs live in `docs/` (Chinese).

## Commands
- `go build ./...` — build all packages
- `go build -o bin/hetu ./cmd/hetu` — build the binary
- `go test -race -shuffle=on -count=1 ./...` — tests
- `go vet ./...` — vet
- `sqlc generate` — regenerate `internal/store/db` from `schema.sql` + `queries/`
- `bin/hetu scan [path]` — index assets under `HETU_LIBRARY_DIR`
- `bin/hetu serve` — run the HTTP server

## Architecture
- `cmd/hetu/main.go` — entrypoint (signals -> cli), <=50 LOC
- `internal/kernel/` — contracts (Plugin, StorageProvider, AssetHandler, Store) + EventBus + JobQueue
- `internal/domain/` — value types (OwnerID, AssetID, Asset, Meta, Entry), no I/O
- `internal/plugins/{dam,nas}/` — capability plugins
- `internal/storage/local/` — local FS StorageProvider (rclone/AList next)
- `internal/asset/image/` — image handler (pure-Go thumbnail; video/3D next)
- `internal/index/` — scan -> extract -> hash -> thumbnail -> upsert chain
- `internal/store/` — SQLite (modernc, no CGO) behind kernel.Store; sqlc code in `store/db`
- `internal/api/` — chi router, mounts plugins under `/api/<name>`
- `internal/app/` — composition root
- `ai/` — Python AI sidecar (Phase 1 stub)
- `deploy/` — Dockerfile + docker-compose (core + rclone/blender/ai sidecars)

## Conventions
- `slog` for all logs; never `log.*` / `fmt.Println`
- `context.Context` first arg for I/O functions
- Errors wrapped with `%w`; typed sentinels in `domain`
- Parse-don't-validate: smart constructors with unexported fields (`domain`)
- Config is env-based (`internal/config/config.go`); `HETU_PLUGINS` picks plugins
- 250 pure LOC ceiling per file
- Timestamps stored as unix INTEGER; `owner_id` reserved for multi-user
- Generated code in `internal/store/db` is never hand-edited (run `sqlc generate`)
