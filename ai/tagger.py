"""Image auto-tagging for `POST /tag` via a WD (waifu-diffusion) ONNX tagger.

The model and its label CSV are downloaded from the Hugging Face repo named by
[Settings.tagger_repo] and cached on disk; inference runs on ONNX Runtime. The
Go contract's ``caption`` field is left empty here — captions come from the
dedicated `POST /caption` endpoint.
"""

from __future__ import annotations

import csv
from dataclasses import dataclass
from typing import TYPE_CHECKING

import numpy as np
import numpy.typing as npt
from PIL import Image

import resolver
from config import onnx_providers, settings
from runtime import Lazy
from schemas import Tag, TagResult

if TYPE_CHECKING:
    from collections.abc import Sequence
    from typing import Any

    from onnxruntime import InferenceSession

_MODEL_FILE = "model.onnx"
_TAGS_FILE = "selected_tags.csv"
_RATING_CATEGORY = 9  # WD label categories: 9=rating, 4=character, 0=general.
_CHARACTER_CATEGORY = 4
_DEFAULT_SIZE = 448


@dataclass(frozen=True, slots=True)
class Label:
    """One WD label: its display name and category code."""

    name: str
    category: int


@dataclass(frozen=True, slots=True)
class TagThresholds:
    """Confidence cutoffs and cap used to select tags from raw scores."""

    general: float
    character: float
    max_tags: int


def _load_labels(path: str) -> list[Label]:
    with open(path, newline="", encoding="utf-8") as handle:  # noqa: PTH123
        return [
            Label(name=row["name"], category=int(row["category"])) for row in csv.DictReader(handle)
        ]


def _preprocess(image: Image.Image, size: int) -> npt.NDArray[np.float32]:
    """WD preprocessing: flatten alpha on white, pad to square, resize, RGB->BGR."""
    canvas = Image.new("RGBA", image.size, (255, 255, 255, 255))
    canvas.alpha_composite(image.convert("RGBA"))
    filled = canvas.convert("RGB")
    side = max(filled.size)
    square = Image.new("RGB", (side, side), (255, 255, 255))
    square.paste(filled, ((side - filled.width) // 2, (side - filled.height) // 2))
    if side != size:
        square = square.resize((size, size), Image.Resampling.BICUBIC)
    array = np.asarray(square, dtype=np.float32)[:, :, ::-1]  # RGB -> BGR
    return np.ascontiguousarray(np.expand_dims(array, axis=0))


def _select_tags(
    scores: npt.NDArray[np.float32], labels: list[Label], cuts: TagThresholds
) -> list[Tag]:
    """Keep general/character labels above their cutoff, ranked, capped at max."""
    chosen: list[Tag] = []
    for label, score in zip(labels, scores.tolist(), strict=False):
        if label.category == _RATING_CATEGORY:
            continue
        cutoff = cuts.character if label.category == _CHARACTER_CATEGORY else cuts.general
        if score >= cutoff:
            chosen.append(Tag(name=label.name.replace("_", " "), confidence=float(score)))
    chosen.sort(key=lambda tag: tag.confidence, reverse=True)
    return chosen[: cuts.max_tags]


class _Tagger:
    """A loaded WD ONNX session with its labels and input geometry."""

    def __init__(self) -> None:
        import onnxruntime as ort
        from huggingface_hub import hf_hub_download

        model_path = hf_hub_download(settings.tagger_repo, _MODEL_FILE)
        self._labels = _load_labels(hf_hub_download(settings.tagger_repo, _TAGS_FILE))
        self._session: InferenceSession = ort.InferenceSession(
            model_path, providers=onnx_providers()
        )
        spec = self._session.get_inputs()[0]
        self._input = spec.name
        self._size = spec.shape[1] if isinstance(spec.shape[1], int) else _DEFAULT_SIZE

    def predict(self, image: Image.Image) -> list[Tag]:
        batch = _preprocess(image, self._size)
        outputs: Sequence[Any] = self._session.run(None, {self._input: batch})
        scores = np.asarray(outputs[0][0], dtype=np.float32)
        cuts = TagThresholds(settings.tag_threshold, settings.tag_char_threshold, settings.max_tags)
        return _select_tags(scores, self._labels, cuts)


_MODEL: Lazy[_Tagger] = Lazy(_Tagger)


def tag_ref(ref: str) -> TagResult:
    """Auto-tag the asset at ``ref``."""
    tags = _MODEL.get().predict(resolver.load_image(ref))
    return TagResult(tags=tags, model=settings.tagger_repo)
