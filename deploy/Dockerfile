# syntax=docker/dockerfile:1

# --- build stage ---
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# modernc.org/sqlite is pure Go, so CGO stays off -> a static binary.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hetu ./cmd/hetu

# --- runtime stage ---
FROM alpine:3.20
RUN adduser -D -u 10001 hetu
# Video (mp4/mov/...) and PDF thumbnails are produced by external tools invoked
# as subprocesses, keeping the kernel CGO-free. They are OPTIONAL: without them
# those assets are still indexed, just without a thumbnail/dimensions. Enable by
# uncommenting (ffmpeg -> ffmpeg+ffprobe; poppler-utils -> pdftoppm):
#   RUN apk add --no-cache ffmpeg poppler-utils
COPY --from=build /out/hetu /usr/local/bin/hetu
USER hetu
ENV HETU_ADDR=":8080" \
    HETU_DATA_DIR="/data" \
    HETU_DB_PATH="/data/hetu.db" \
    HETU_LIBRARY_DIR="/library" \
    HETU_PLUGINS="dam,nas"
VOLUME ["/data", "/library"]
EXPOSE 8080
ENTRYPOINT ["hetu"]
CMD ["serve"]
