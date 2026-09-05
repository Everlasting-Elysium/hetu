"""Image captioning for `POST /caption` via BLIP.

The BLIP model named by [Settings.caption_model] is downloaded and cached on
disk, then loaded once on first request and run on the configured device.
"""
# transformers' BLIP model/processor stubs are incomplete for `.to(device)` and the
# processor kwargs; relax those two checks for this model-glue file only.
# pyright: reportArgumentType=false, reportCallIssue=false

from __future__ import annotations

from typing import TYPE_CHECKING

import resolver
from config import settings
from runtime import Lazy
from schemas import CaptionResult

if TYPE_CHECKING:
    from PIL import Image


class _Captioner:
    """A loaded BLIP model and its processor."""

    def __init__(self) -> None:
        import torch
        from transformers import BlipForConditionalGeneration, BlipProcessor

        self._torch = torch
        self._processor = BlipProcessor.from_pretrained(settings.caption_model)
        self._model = (
            BlipForConditionalGeneration.from_pretrained(settings.caption_model)
            .to(settings.device)
            .eval()
        )

    def caption(self, image: Image.Image) -> str:
        inputs = self._processor(images=image.convert("RGB"), return_tensors="pt").to(
            settings.device
        )
        with self._torch.no_grad():
            tokens = self._model.generate(**inputs, max_new_tokens=settings.caption_max_tokens)
        return str(self._processor.decode(tokens[0], skip_special_tokens=True)).strip()


_MODEL: Lazy[_Captioner] = Lazy(_Captioner)


def caption_ref(ref: str) -> CaptionResult:
    """Describe the asset at ``ref`` in natural language."""
    text = _MODEL.get().caption(resolver.load_image(ref))
    return CaptionResult(caption=text, model=settings.caption_model)
