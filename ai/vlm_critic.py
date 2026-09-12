"""Interleaved multi-image critique for `POST /compare` via a small VLM.

The VLM named by [Settings.vlm_model] (e.g. ``HuggingFaceTB/SmolVLM-500M-Instruct``)
is loaded once on first request and run on CPU in float32. It sees BOTH images in
a single forward pass — an honest "compare these two" — and is asked for an overall
summary plus one actionable note per requested dimension. Its free-text reply is
parsed leniently into [CompareResult]; unparseable output degrades to a
summary-only result rather than failing.

The endpoint distinguishes two states (see server.compare_assets): an empty
[Settings.vlm_model] means *unconfigured* and returns 501 without loading
anything, while a configured model whose load or inference raises propagates the
error to a 500.
"""
# transformers' processor/model stubs are incomplete for the chat-template and
# generate kwargs and do not expose `.generate` on the AutoModel base type;
# relax those checks for this model-glue file only.
# pyright: reportArgumentType=false, reportCallIssue=false, reportAttributeAccessIssue=false

from __future__ import annotations

import re
from typing import TYPE_CHECKING

import resolver
from config import settings
from runtime import Lazy
from schemas import CompareResult

if TYPE_CHECKING:
    from collections.abc import Sequence

    from PIL import Image

_MAX_NEW_TOKENS = 128
# A dimension label the model is asked to emit, e.g. "color: ...", "tone - ...",
# tolerant of a plain colon, a full-width colon, or a dash, anywhere on a line.
_LABEL_SUFFIX = r"\s*[:：-]\s*"  # noqa: RUF001 — full-width colon is intentional


def _build_prompt(dimensions: Sequence[str]) -> str:
    """Ask for an overall summary plus one labelled note per dimension.

    The dimension names are echoed as line labels so [_parse_critique] can split
    the reply back into a per-dimension mapping.
    """
    if not dimensions:
        return (
            "Compare the first image (reference) with the second image (target) "
            "and give a short overall summary of how the target differs."
        )
    lines = "\n".join(f"- {dim}: <one concrete, actionable suggestion>" for dim in dimensions)
    return (
        "Compare the first image (reference) with the second image (target). "
        "Give a one-sentence overall summary, then address each dimension below "
        "on its own line, starting with the dimension name and a colon:\n"
        f"{lines}"
    )


def _parse_critique(raw: str, dimensions: Sequence[str]) -> tuple[str, dict[str, str]]:
    """Split a free-text reply into (summary, per-dimension notes).

    Finds each requested dimension introduced as a label (``color:``, ``tone -``).
    Text before the first label is the summary; each label's text runs to the next
    label. If no label is found the whole reply becomes the summary and the mapping
    is empty — a lenient degradation, never an error.
    """
    text = raw.strip()
    if not text:
        return "", {}
    hits: list[tuple[int, int, str]] = []
    for dim in dimensions:
        match = re.search(rf"(?i)\b{re.escape(dim)}\b{_LABEL_SUFFIX}", text)
        if match:
            hits.append((match.start(), match.end(), dim))
    if not hits:
        return text, {}
    hits.sort()
    summary = text[: hits[0][0]].strip()
    notes: dict[str, str] = {}
    for i, (_, content_start, dim) in enumerate(hits):
        end = hits[i + 1][0] if i + 1 < len(hits) else len(text)
        note = text[content_start:end].strip()
        if note:
            notes[dim] = note
    return summary, notes


class _VLMCritic:
    """A loaded interleaved-multi-image VLM and its processor (CPU, float32)."""

    def __init__(self) -> None:
        import torch
        from transformers import AutoModelForImageTextToText, AutoProcessor

        self._torch = torch
        self._processor = AutoProcessor.from_pretrained(settings.vlm_model)
        self._model = (
            AutoModelForImageTextToText.from_pretrained(settings.vlm_model, dtype=torch.float32)
            .to(settings.device)
            .eval()
        )

    def critique(
        self, image_a: Image.Image, image_b: Image.Image, dimensions: Sequence[str]
    ) -> str:
        messages = [
            {
                "role": "user",
                "content": [
                    {"type": "image"},
                    {"type": "image"},
                    {"type": "text", "text": _build_prompt(dimensions)},
                ],
            }
        ]
        prompt = self._processor.apply_chat_template(messages, add_generation_prompt=True)
        inputs = self._processor(
            text=prompt,
            images=[image_a.convert("RGB"), image_b.convert("RGB")],
            return_tensors="pt",
        ).to(settings.device)
        with self._torch.no_grad():
            generated = self._model.generate(**inputs, max_new_tokens=_MAX_NEW_TOKENS)
        # generate() echoes the prompt tokens; slice them off for a clean reply.
        reply = generated[:, inputs["input_ids"].shape[1] :]
        decoded = self._processor.batch_decode(reply, skip_special_tokens=True)
        return str(decoded[0]).strip()


_MODEL: Lazy[_VLMCritic] = Lazy(_VLMCritic)


def compare_refs(ref_a: str, ref_b: str, dimensions: list[str]) -> CompareResult:
    """Critique the two referenced images across ``dimensions`` with the VLM."""
    image_a = resolver.load_image(ref_a)
    image_b = resolver.load_image(ref_b)
    raw = _MODEL.get().critique(image_a, image_b, dimensions)
    summary, notes = _parse_critique(raw, dimensions)
    return CompareResult(summary=summary, dimensions=notes, model=settings.vlm_model)
