"""Unit tests for the pure post-processing helpers and the ref resolver."""
# Tests intentionally exercise module-internal helpers.
# pyright: reportPrivateUsage=false

from __future__ import annotations

from typing import TYPE_CHECKING

import numpy as np
import pytest
from PIL import Image

from ocr import _to_block
from resolver import RefResolveError, load_image, load_image_or_text
from tagger import Label, TagThresholds, _select_tags

if TYPE_CHECKING:
    from pathlib import Path


def test_select_tags_skips_rating_and_ranks_by_confidence() -> None:
    labels = [Label("rating_safe", 9), Label("cat", 0), Label("dog", 0), Label("char_name", 4)]
    scores = np.array([0.99, 0.9, 0.2, 0.8], dtype=np.float32)
    cuts = TagThresholds(general=0.35, character=0.75, max_tags=10)

    tags = _select_tags(scores, labels, cuts)

    # rating dropped (category 9); dog dropped (0.2 < 0.35); underscores -> spaces.
    assert [tag.name for tag in tags] == ["cat", "char name"]
    assert tags[0].confidence == pytest.approx(0.9, abs=1e-6)


def test_select_tags_caps_at_max() -> None:
    labels = [Label(f"t{i}", 0) for i in range(5)]
    scores = np.array([0.9, 0.8, 0.7, 0.6, 0.5], dtype=np.float32)
    cuts = TagThresholds(general=0.1, character=0.1, max_tags=2)

    tags = _select_tags(scores, labels, cuts)

    assert [tag.name for tag in tags] == ["t0", "t1"]


def test_to_block_computes_axis_aligned_bbox() -> None:
    quad = [[10, 20], [50, 22], [48, 60], [12, 58]]

    block = _to_block(quad, "hello", 0.95)

    assert block.text == "hello"
    assert block.confidence == pytest.approx(0.95)
    assert block.bbox == (10, 20, 50, 60)


def test_load_image_missing_ref_raises() -> None:
    with pytest.raises(RefResolveError):
        load_image("/no/such/file.png")


def test_load_image_or_text_falls_back_to_text() -> None:
    assert load_image_or_text("a red sports car") == "a red sports car"


def test_load_image_reads_local_file(tmp_path: Path) -> None:
    path = tmp_path / "swatch.png"
    Image.new("RGB", (4, 4), (255, 0, 0)).save(path)

    image = load_image(str(path))

    assert image.size == (4, 4)
