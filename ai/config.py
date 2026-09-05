"""Environment-driven configuration for the AI sidecar.

All settings are read from ``HETU_AI_*`` environment variables. Model weights are
cached on disk under [Settings.cache_dir]; inference runs on CPU unless
[Settings.device] selects ``cuda``.
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import Literal

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict

Device = Literal["cpu", "cuda"]


class Settings(BaseSettings):
    """Sidecar configuration parsed from the ``HETU_AI_`` environment."""

    model_config = SettingsConfigDict(env_prefix="HETU_AI_", frozen=True)

    cache_dir: Path = Field(default=Path.home() / ".cache" / "hetu-ai")
    device: Device = "cpu"

    clip_model: str = "openai/clip-vit-base-patch32"
    tagger_repo: str = "SmilingWolf/wd-v1-4-moat-tagger-v2"
    caption_model: str = "Salesforce/blip-image-captioning-base"

    # Tag selection thresholds; general labels are noisier than character labels.
    tag_threshold: float = 0.35
    tag_char_threshold: float = 0.75
    max_tags: int = 30

    caption_max_tokens: int = 40
    http_timeout: float = 20.0
    # Upper bound on concurrent (CPU/GPU-bound) inferences run off the event loop.
    max_concurrency: int = 2


settings = Settings()


def apply_runtime_env() -> None:
    """Point the Hugging Face cache at [Settings.cache_dir] before models load.

    ``transformers`` and ``huggingface_hub`` read ``HF_HOME`` at import time; the
    sidecar imports them lazily, so calling this at startup is sufficient.
    """
    settings.cache_dir.mkdir(parents=True, exist_ok=True)
    os.environ.setdefault("HF_HOME", str(settings.cache_dir))


def onnx_providers() -> list[str]:
    """ONNX Runtime execution providers for the configured device."""
    if settings.device == "cuda":
        return ["CUDAExecutionProvider", "CPUExecutionProvider"]
    return ["CPUExecutionProvider"]
