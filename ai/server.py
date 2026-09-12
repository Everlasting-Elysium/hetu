"""hetu AI sidecar: local-first CLIP, tagging, captioning, and OCR over HTTP.

Implements the versioned contract the Go core speaks (internal/ai/types.go):

    GET  /health -> {"ok": true}
    POST /embed  -> {"vector": [...], "dim": N, "model": "..."}
    POST /tag    -> {"tags": [...], "caption": "", "model": "..."}
    POST /caption-> {"caption": "...", "model": "..."}
    POST /ocr    -> {"text": "...", "blocks": [...], "model": "..."}
    POST /compare-> {"summary": "...", "dimensions": {...}, "model": "..."}

Every inference POST body is an [AssetRef] the sidecar resolves locally (``/compare``
carries two refs). Models load lazily on first use, so ``/health`` is ready as soon
as the process is up. Inference runs off the event loop, bounded by
[runtime.run_inference].
"""

from __future__ import annotations

from contextlib import asynccontextmanager
from typing import TYPE_CHECKING

from fastapi import FastAPI, HTTPException, Request, status
from fastapi.responses import JSONResponse

import caption
import embed
import ocr
import tagger
import vlm_critic
from config import apply_runtime_env, settings
from resolver import RefResolveError
from runtime import run_inference
from schemas import (
    AssetRef,
    CaptionResult,
    CompareRequest,
    CompareResult,
    EmbedResult,
    Health,
    OCRResult,
    TagResult,
)

if TYPE_CHECKING:
    from collections.abc import AsyncGenerator


@asynccontextmanager
async def _lifespan(_app: FastAPI) -> AsyncGenerator[None]:
    apply_runtime_env()
    yield


async def on_ref_error(_request: Request, exc: Exception) -> JSONResponse:
    """Map a bad/unreadable ref to HTTP 400 (Go maps 400 -> terminal KindInvalid)."""
    return JSONResponse(status_code=status.HTTP_400_BAD_REQUEST, content={"detail": str(exc)})


app = FastAPI(title="hetu-ai", version="0.1.0", lifespan=_lifespan)
app.add_exception_handler(RefResolveError, on_ref_error)


@app.get("/health")
async def health() -> Health:
    """Report readiness; true as soon as the process is up (models load lazily)."""
    return Health(ok=True)


@app.post("/embed")
async def embed_asset(req: AssetRef) -> EmbedResult:
    """Return a CLIP embedding for the referenced asset (or text query)."""
    return await run_inference(lambda: embed.embed_ref(req.ref))


@app.post("/tag")
async def tag_asset(req: AssetRef) -> TagResult:
    """Return auto-tags for the referenced image."""
    return await run_inference(lambda: tagger.tag_ref(req.ref))


@app.post("/caption")
async def caption_asset(req: AssetRef) -> CaptionResult:
    """Return a natural-language caption for the referenced image."""
    return await run_inference(lambda: caption.caption_ref(req.ref))


@app.post("/ocr")
async def ocr_asset(req: AssetRef) -> OCRResult:
    """Extract text and per-block boxes from the referenced image."""
    return await run_inference(lambda: ocr.ocr_ref(req.ref))


@app.post("/compare")
async def compare_assets(req: CompareRequest) -> CompareResult:
    """Critique two images across the requested dimensions with the VLM.

    Returns 501 when no VLM is configured (empty [Settings.vlm_model]) — nothing
    is loaded. A configured model whose load or inference fails propagates to a
    500; an unresolvable ref is a 400 (handled by [on_ref_error]).
    """
    if not settings.vlm_model:
        raise HTTPException(
            status_code=status.HTTP_501_NOT_IMPLEMENTED,
            detail="HETU_AI_VLM_MODEL not configured",
        )
    return await run_inference(
        lambda: vlm_critic.compare_refs(req.ref_a, req.ref_b, req.dimensions)
    )
