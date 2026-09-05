"""Text extraction for `POST /ocr` via RapidOCR (PP-OCRv4 ONNX models).

RapidOCR bundles the PP-OCRv4 detection/recognition ONNX models in its wheel, so
OCR needs no separate download and runs entirely on ONNX Runtime. The engine is
constructed once on first request.
"""
# RapidOCR's __call__ return is loosely typed; relax argument typing for this
# model-glue file only. The bbox math in _to_block is covered by unit tests.
# pyright: reportArgumentType=false

from __future__ import annotations

from typing import TYPE_CHECKING

import numpy as np

import resolver
from runtime import Lazy
from schemas import OCRBlock, OCRResult

if TYPE_CHECKING:
    from collections.abc import Sequence

    from PIL import Image

_MODEL_NAME = "rapidocr-onnxruntime(PP-OCRv4)"


def _to_block(box: Sequence[Sequence[float]], text: str, score: float) -> OCRBlock:
    """Convert a RapidOCR quad (4 [x,y] points) to a [OCRBlock] with an axis bbox."""
    xs = [point[0] for point in box]
    ys = [point[1] for point in box]
    bbox = (int(min(xs)), int(min(ys)), int(max(xs)), int(max(ys)))
    return OCRBlock(text=text, confidence=float(score), bbox=bbox)


class _OCR:
    """A constructed RapidOCR engine."""

    def __init__(self) -> None:
        from rapidocr_onnxruntime import RapidOCR

        self._engine = RapidOCR()

    def read(self, image: Image.Image) -> list[OCRBlock]:
        array = np.ascontiguousarray(np.asarray(image.convert("RGB"))[:, :, ::-1])  # RGB -> BGR
        result, _ = self._engine(array)
        if not isinstance(result, list):  # None when no text is detected
            return []
        return [_to_block(item[0], str(item[1]), float(item[2])) for item in result]


_MODEL: Lazy[_OCR] = Lazy(_OCR)


def ocr_ref(ref: str) -> OCRResult:
    """Extract text from the asset at ``ref``."""
    blocks = _MODEL.get().read(resolver.load_image(ref))
    text = "\n".join(block.text for block in blocks)
    return OCRResult(text=text, blocks=blocks, model=_MODEL_NAME)
