"""hetu AI sidecar (stub).

Phase 1 implements local models here: CLIP embeddings for semantic/visual
search, an image tagger, a captioner, face detection, and OCR. For now this is
a contract stub so the Go core can be wired against a stable HTTP surface.
"""

from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI(title="hetu-ai", version="0.0.0")


class AssetRef(BaseModel):
    # A storage path or URL the core can resolve. Phase 1 fixes the transport.
    ref: str


@app.get("/health")
def health() -> dict[str, bool]:
    return {"ok": True}


@app.post("/embed", status_code=501)
def embed(_req: AssetRef) -> dict[str, str]:
    # Phase 1: return a CLIP embedding vector for the referenced asset.
    return {"status": "not_implemented"}


@app.post("/tag", status_code=501)
def tag(_req: AssetRef) -> dict[str, str]:
    # Phase 1: return auto-tags + a caption for the referenced asset.
    return {"status": "not_implemented"}
