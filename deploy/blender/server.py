#!/usr/bin/env python3
"""HTTP wrapper around Blender headless rendering.

POST /render  (multipart form field "model")  ->  512x512 PNG thumbnail.

Each request runs `blender -b -P render.py -- <in> <out>` in a fresh temp
directory. The model format is sniffed from magic bytes, so callers need not
send a filename or extension.

Env:
  BLENDER_LISTEN  host:port to bind (default ":9090")
  BLENDER_BIN     blender executable (default "blender")
"""
import os
import subprocess
import tempfile

from flask import Flask, Response, abort, request

app = Flask(__name__)

RENDER_PY = os.path.join(os.path.dirname(os.path.abspath(__file__)), "render.py")
BLENDER_BIN = os.environ.get("BLENDER_BIN", "blender")
RENDER_TIMEOUT_S = 120


def sniff_ext(data: bytes) -> str:
    """Map a model file's leading bytes to a Blender-importable extension."""
    if data[:4] == b"glTF":
        return ".glb"
    if data[:3] == b"ply":
        return ".ply"
    if data[:18] == b"Kaydara FBX Binary":
        return ".fbx"
    if data[:8] == b"PXR-USDC" or data[:5] == b"#usda":
        return ".usd"
    if data[:4] == b"PK\x03\x04":
        return ".usdz"
    head = data[:512].lstrip()
    if head[:1] == b"{":
        return ".gltf"
    if head[:5].lower() == b"solid":
        return ".stl"
    return ".obj"


@app.post("/render")
def render() -> Response:
    upload = request.files.get("model")
    if upload is None:
        abort(400, "missing 'model' file")
    data = upload.read()
    if not data:
        abort(400, "empty 'model' file")
    with tempfile.TemporaryDirectory() as tmp:
        src = os.path.join(tmp, "model" + sniff_ext(data))
        out = os.path.join(tmp, "thumb.png")
        with open(src, "wb") as f:
            f.write(data)
        proc = subprocess.run(
            [BLENDER_BIN, "-b", "-P", RENDER_PY, "--", src, out],
            capture_output=True,
            text=True,
            timeout=RENDER_TIMEOUT_S,
            check=False,
        )
        if proc.returncode != 0 or not os.path.exists(out):
            app.logger.error("blender render failed: %s", proc.stderr.strip())
            abort(500, "render failed")
        with open(out, "rb") as f:
            png = f.read()
    return Response(png, mimetype="image/png")


def _addr() -> tuple[str, int]:
    host, _, port = os.environ.get("BLENDER_LISTEN", ":9090").rpartition(":")
    return host or "0.0.0.0", int(port or "9090")


if __name__ == "__main__":
    listen_host, listen_port = _addr()
    app.run(host=listen_host, port=listen_port)
