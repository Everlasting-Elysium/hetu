# syntax=docker/dockerfile:1
FROM python:3.12-slim

# opencv (pulled in by rapidocr-onnxruntime) needs libGL + glib at runtime.
RUN apt-get update \
 && apt-get install -y --no-install-recommends libgl1 libglib2.0-0 \
 && rm -rf /var/lib/apt/lists/*

COPY --from=ghcr.io/astral-sh/uv:0.12 /uv /bin/uv

WORKDIR /app
ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never

# Dependency layer first (manifests change rarely) so image rebuilds stay cheap.
COPY pyproject.toml uv.lock ./
RUN --mount=type=cache,target=/root/.cache/uv uv sync --frozen --no-dev

# Application code last.
COPY . .

# CPU by default; on-disk model cache lives on a mounted volume so weights are
# downloaded once. Set HETU_AI_DEVICE=cuda (with a CUDA base image) for GPU.
ENV PATH="/app/.venv/bin:$PATH" \
    HETU_AI_CACHE_DIR=/models
VOLUME ["/models"]
EXPOSE 8091
CMD ["uvicorn", "server:app", "--host", "0.0.0.0", "--port", "8091"]
