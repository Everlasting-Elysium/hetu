"""Wire types for the AI sidecar HTTP contract (see internal/ai/types.go).

Every field name and JSON shape here is fixed by the Go client's structs; these
models MUST serialize to bodies that deserialize into internal/ai unchanged. The
contract version is [CONTRACT_VERSION] (Go: ai.ContractVersion).
"""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field

# Mirrors ai.ContractVersion / ai.HeaderContractVersion in internal/ai/types.go.
CONTRACT_VERSION = "v1"
CONTRACT_HEADER = "X-Hetu-AI-Contract"


class AssetRef(BaseModel):
    """Request body of every inference endpoint: a path or URL to resolve."""

    model_config = ConfigDict(frozen=True)

    ref: str


class Health(BaseModel):
    """`GET /health` response (Go: ai.Health)."""

    model_config = ConfigDict(frozen=True)

    ok: bool


class Tag(BaseModel):
    """One predicted label with confidence in [0, 1] (Go: ai.Tag)."""

    model_config = ConfigDict(frozen=True)

    name: str
    confidence: float


class TagResult(BaseModel):
    """`POST /tag` response (Go: ai.TagResult)."""

    model_config = ConfigDict(frozen=True)

    tags: list[Tag]
    caption: str = ""
    model: str


class EmbedResult(BaseModel):
    """`POST /embed` response: a CLIP vector and its length (Go: ai.EmbedResult)."""

    model_config = ConfigDict(frozen=True)

    vector: list[float]
    dim: int
    model: str


class CaptionResult(BaseModel):
    """`POST /caption` response (Go: ai.CaptionResult)."""

    model_config = ConfigDict(frozen=True)

    caption: str
    model: str


class OCRBlock(BaseModel):
    """One detected text run with confidence and bbox [x0,y0,x1,y1] (Go: ai.OCRBlock)."""

    model_config = ConfigDict(frozen=True)

    text: str
    confidence: float
    bbox: tuple[int, int, int, int]


class OCRResult(BaseModel):
    """`POST /ocr` response (Go: ai.OCRResult)."""

    model_config = ConfigDict(frozen=True)

    text: str
    blocks: list[OCRBlock] = Field(default_factory=list)
    model: str
