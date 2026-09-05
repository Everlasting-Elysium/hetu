"""CLIP embeddings for `POST /embed`.

Produces an L2-normalized CLIP vector for the referenced asset — an image when
the ref resolves to one, otherwise a text query (docs/ai-and-3d.md: CLIP encodes
images and text into one space for semantic and visual search). The model is
loaded once, lazily, on first request.
"""
# transformers' CLIPModel/CLIPProcessor stubs are incomplete for `.to(device)` and
# the processor kwargs (return_tensors/padding/truncation); relax those two checks
# for this model-glue file only. Pure logic lives in typed helpers and schemas.
# pyright: reportArgumentType=false, reportCallIssue=false

from __future__ import annotations

from typing import TYPE_CHECKING

import resolver
from config import settings
from runtime import Lazy
from schemas import EmbedResult

if TYPE_CHECKING:
    import torch
    from PIL import Image


def _normalize(features: torch.Tensor) -> list[float]:
    """L2-normalize the first (batch) row and return it as a plain list."""
    vector = features[0]
    return (vector / vector.norm()).cpu().tolist()


class _Clip:
    """A loaded CLIP model and its processor."""

    def __init__(self) -> None:
        import torch
        from transformers import CLIPModel, CLIPProcessor

        self._torch = torch
        self._model = CLIPModel.from_pretrained(settings.clip_model).to(settings.device).eval()
        self._processor = CLIPProcessor.from_pretrained(settings.clip_model)

    def embed_image(self, image: Image.Image) -> list[float]:
        inputs = self._processor(images=image.convert("RGB"), return_tensors="pt").to(
            settings.device
        )
        with self._torch.no_grad():
            features = self._model.get_image_features(**inputs)
        return _normalize(features)

    def embed_text(self, text: str) -> list[float]:
        inputs = self._processor(
            text=[text], return_tensors="pt", padding=True, truncation=True
        ).to(settings.device)
        with self._torch.no_grad():
            features = self._model.get_text_features(**inputs)
        return _normalize(features)


_MODEL: Lazy[_Clip] = Lazy(_Clip)


def embed_ref(ref: str) -> EmbedResult:
    """Embed the asset or text query at ``ref`` into a CLIP vector."""
    source = resolver.load_image_or_text(ref)
    model = _MODEL.get()
    vector = model.embed_text(source) if isinstance(source, str) else model.embed_image(source)
    return EmbedResult(vector=vector, dim=len(vector), model=settings.clip_model)
