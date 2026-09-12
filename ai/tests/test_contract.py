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
import vlm_critic
from config import Settings
from resolver import RefResolveError
from schemas import (
    CONTRACT_VERSION,
    CaptionResult,
    CompareResult,
    EmbedResult,
    OCRBlock,
    OCRResult,
    Tag,
    TagResult,
)

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


def test_contract_version_is_v2() -> None:
    # The Go side asserts its own ai.ContractVersion == "v2" independently; both
    # must be bumped together (manual lockstep, no cross-language read).
    assert CONTRACT_VERSION == "v2"


def test_compare_matches_contract(client: TestClient, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(server, "settings", Settings(vlm_model="fake-vlm"))
    monkeypatch.setattr(
        vlm_critic,
        "compare_refs",
        lambda _a, _b, _dims: CompareResult(
            summary="cooler and darker",
            dimensions={"color": "warm it up"},
            model="fake-vlm",
        ),
    )
    body = client.post(
        "/compare", json={"ref_a": "a.png", "ref_b": "b.png", "dimensions": ["color"]}
    ).json()
    assert set(body) == {"summary", "dimensions", "model"}
    assert body["summary"] == "cooler and darker"
    assert body["dimensions"] == {"color": "warm it up"}
    assert body["model"] == "fake-vlm"


def test_compare_unconfigured_returns_501(
    client: TestClient, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(server, "settings", Settings(vlm_model=""))
    calls: list[bool] = []

    def spy(_a: str, _b: str, _dims: list[str]) -> CompareResult:
        calls.append(True)
        return CompareResult(summary="", dimensions={}, model="")

    monkeypatch.setattr(vlm_critic, "compare_refs", spy)
    response = client.post(
        "/compare", json={"ref_a": "a.png", "ref_b": "b.png", "dimensions": ["color"]}
    )
    assert response.status_code == 501
    assert calls == []  # unconfigured must not attempt any model work


def test_compare_load_failure_returns_500(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(server, "settings", Settings(vlm_model="fake-vlm"))

    def boom(_a: str, _b: str, _dims: list[str]) -> CompareResult:
        raise RuntimeError("model failed to load")

    monkeypatch.setattr(vlm_critic, "compare_refs", boom)
    # A configured model that fails is a real error (500), not "unimplemented"
    # (501); raise_server_exceptions=False so the client returns the response.
    with TestClient(server.app, raise_server_exceptions=False) as failing_client:
        response = failing_client.post(
            "/compare", json={"ref_a": "a.png", "ref_b": "b.png", "dimensions": ["color"]}
        )
    assert response.status_code == 500
