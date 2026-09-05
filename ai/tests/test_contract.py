"""Contract tests: endpoint JSON must match the Go client in internal/ai.

The model functions are monkeypatched so these run without downloading weights;
what they assert is the wire shape (exact keys and value types) that the Go
structs in internal/ai/types.go decode.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import pytest
from fastapi.testclient import TestClient

import caption
import embed
import ocr
import server
import tagger
from resolver import RefResolveError
from schemas import CaptionResult, EmbedResult, OCRBlock, OCRResult, Tag, TagResult

if TYPE_CHECKING:
    from collections.abc import Iterator


@pytest.fixture
def client() -> Iterator[TestClient]:
    with TestClient(server.app) as test_client:
        yield test_client


def test_health_returns_ok(client: TestClient) -> None:
    response = client.get("/health")
    assert response.status_code == 200
    assert response.json() == {"ok": True}


def test_embed_matches_contract(client: TestClient, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(
        embed, "embed_ref", lambda _ref: EmbedResult(vector=[0.1, 0.2, 0.3], dim=3, model="clip")
    )
    response = client.post("/embed", json={"ref": "photo.jpg"})
    body = response.json()
    assert response.status_code == 200
    assert set(body) == {"vector", "dim", "model"}
    assert body["dim"] == 3
    assert len(body["vector"]) == 3
    assert body["model"] == "clip"


def test_tag_matches_contract(client: TestClient, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(
        tagger,
        "tag_ref",
        lambda _ref: TagResult(tags=[Tag(name="cat", confidence=0.98)], model="wd"),
    )
    body = client.post("/tag", json={"ref": "photo.jpg"}).json()
    assert set(body) == {"tags", "caption", "model"}
    assert body["tags"] == [{"name": "cat", "confidence": 0.98}]
    assert body["caption"] == ""
    assert body["model"] == "wd"


def test_caption_matches_contract(client: TestClient, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(
        caption, "caption_ref", lambda _ref: CaptionResult(caption="a photo", model="blip")
    )
    body = client.post("/caption", json={"ref": "photo.jpg"}).json()
    assert set(body) == {"caption", "model"}
    assert body["caption"] == "a photo"
    assert body["model"] == "blip"


def test_ocr_matches_contract(client: TestClient, monkeypatch: pytest.MonkeyPatch) -> None:
    result = OCRResult(
        text="hello",
        blocks=[OCRBlock(text="hello", confidence=0.9, bbox=(1, 2, 3, 4))],
        model="rapidocr",
    )
    monkeypatch.setattr(ocr, "ocr_ref", lambda _ref: result)
    body = client.post("/ocr", json={"ref": "scan.png"}).json()
    assert set(body) == {"text", "blocks", "model"}
    assert body["text"] == "hello"
    assert body["blocks"] == [{"text": "hello", "confidence": 0.9, "bbox": [1, 2, 3, 4]}]


def test_bad_ref_returns_400(client: TestClient, monkeypatch: pytest.MonkeyPatch) -> None:
    def boom(_ref: str) -> TagResult:
        raise RefResolveError("no such asset: missing.jpg")

    monkeypatch.setattr(tagger, "tag_ref", boom)
    response = client.post("/tag", json={"ref": "missing.jpg"})
    assert response.status_code == 400
    assert "detail" in response.json()


def test_missing_ref_field_is_rejected(client: TestClient) -> None:
    assert client.post("/embed", json={}).status_code == 422
