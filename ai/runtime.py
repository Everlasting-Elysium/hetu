"""Lazy model loading and bounded off-loop inference.

Models are large and slow to construct, so each is wrapped in a thread-safe
[Lazy] holder that builds it once, on first use. Inference itself is CPU/GPU
bound and blocking, so it runs in a worker thread via [run_inference], capped by
a shared [anyio.CapacityLimiter] so concurrent requests cannot exhaust memory.
"""

from __future__ import annotations

import threading
from typing import TYPE_CHECKING

import anyio
import anyio.to_thread

from config import settings

if TYPE_CHECKING:
    from collections.abc import Callable

_LIMITER = anyio.CapacityLimiter(settings.max_concurrency)


class Lazy[T]:
    """A value built once, on first [get], safe under concurrent callers."""

    def __init__(self, factory: Callable[[], T]) -> None:
        """Hold ``factory`` to build the value on the first [get]."""
        self._factory = factory
        self._lock = threading.Lock()
        self._value: T | None = None

    def get(self) -> T:
        """Return the value, constructing it on the first call."""
        cached = self._value
        if cached is not None:
            return cached
        with self._lock:
            created = self._value
            if created is None:
                created = self._factory()
                self._value = created
            return created


async def run_inference[T](fn: Callable[[], T]) -> T:
    """Run blocking inference ``fn`` in a worker thread, bounded by the limiter."""
    return await anyio.to_thread.run_sync(fn, limiter=_LIMITER)
