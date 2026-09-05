"""Resolve an [AssetRef] ``ref`` to the bytes the models consume.

Local-first (docs/ai-and-3d.md): a ``ref`` is a local storage path or an
``http(s)`` URL. A ref that resolves to neither is a client error
([RefResolveError] -> HTTP 400), except for [load_image_or_text], where an
unresolvable ref is treated as a CLIP text query.
"""

from __future__ import annotations

import io
from pathlib import Path

import httpx
from PIL import Image

from config import settings

_URL_SCHEMES = ("http://", "https://")


class RefResolveError(Exception):
    """A ``ref`` could not be resolved to a readable image."""


def _is_url(ref: str) -> bool:
    return ref.startswith(_URL_SCHEMES)


def _fetch(url: str) -> bytes:
    try:
        response = httpx.get(url, timeout=settings.http_timeout, follow_redirects=True)
        response.raise_for_status()
    except httpx.HTTPError as exc:
        raise RefResolveError(f"fetch {url}: {exc}") from exc
    return response.content


def _decode(source: str | bytes, ref: str) -> Image.Image:
    try:
        # Pillow raises UnidentifiedImageError (an OSError) for unknown formats
        # and OSError for truncated data.
        image = Image.open(source if isinstance(source, str) else io.BytesIO(source))
        image.load()
    except OSError as exc:
        raise RefResolveError(f"decode {ref}: {exc}") from exc
    return image


def load_image(ref: str) -> Image.Image:
    """Resolve ``ref`` to an image, raising [RefResolveError] if impossible."""
    if _is_url(ref):
        return _decode(_fetch(ref), ref)
    if Path(ref).is_file():
        return _decode(ref, ref)
    raise RefResolveError(f"no such asset: {ref}")


def load_image_or_text(ref: str) -> Image.Image | str:
    """Resolve ``ref`` to an image, or fall back to treating it as a text query."""
    if _is_url(ref) or Path(ref).is_file():
        return load_image(ref)
    return ref
