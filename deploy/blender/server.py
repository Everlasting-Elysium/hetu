#!/usr/bin/env python3
"""HTTP wrapper around Blender headless rendering and conversion.

POST /render   (multipart form field "model")  ->  512x512 PNG thumbnail (#12).
POST /convert  (multipart "model", ?ext=<fmt>) ->  self-contained GLB (#51).

Each request runs `blender -b -P <script> -- <in> <out>` in a fresh temp
directory. /render sniffs the format from magic bytes; /convert prefers the
caller-supplied ?ext= (the backend knows the true extension), which is essential
for binary STL that carries no reliable signature, and falls back to sniffing.

Env:
  BLENDER_LISTEN  host:port to bind (default ":9090")
  BLENDER_BIN     blender executable (default "blender")
"""
import os
import subprocess
import tempfile

from flask import Flask, Response, abort, request

app = Flask(__name__)

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
RENDER_PY = os.path.join(SCRIPT_DIR, "render.py")
CONVERT_PY = os.path.join(SCRIPT_DIR, "convert.py")
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


def _read_upload() -> bytes:
    """Return the non-empty "model" upload bytes, aborting 400 otherwise."""
    upload = request.files.get("model")
    if upload is None:
        abort(400, "missing 'model' file")
    data = upload.read()
    if not data:
        abort(400, "empty 'model' file")
    return data


def _run(script: str, data: bytes, ext: str, out_name: str) -> bytes:
    """Run a Blender script over the model bytes and return the output file.

    Writes the upload to a temp dir with extension `ext` (so Blender picks the
    right importer), runs `blender -b -P <script> -- <in> <out>`, and returns the
    produced file's bytes. Aborts 500 on any Blender failure or missing output.
    """
    with tempfile.TemporaryDirectory() as tmp:
        src = os.path.join(tmp, "model" + ext)
        out = os.path.join(tmp, out_name)
        with open(src, "wb") as f:
            f.write(data)
        proc = subprocess.run(
            [BLENDER_BIN, "-b", "-P", script, "--", src, out],
            capture_output=True,
            text=True,
            timeout=RENDER_TIMEOUT_S,
            check=False,
        )
        if proc.returncode != 0 or not os.path.exists(out) or os.path.getsize(out) == 0:
            app.logger.error("blender %s failed: %s",
                             os.path.basename(script), proc.stderr.strip())
            abort(500, "blender job failed")
        with open(out, "rb") as f:
            return f.read()


@app.post("/render")
def render() -> Response:
    data = _read_upload()
    png = _run(RENDER_PY, data, sniff_ext(data), "thumb.png")
    return Response(png, mimetype="image/png")


@app.post("/convert")
def convert() -> Response:
    data = _read_upload()
    raw = request.args.get("ext", "").strip().lstrip(".").lower()
    # Defense in depth: an extension is always alphanumeric (obj/fbx/stl/...), so
    # reject anything else before it reaches a filesystem path (no separators, no
    # traversal). The backend already sends a clean value.
    if raw and not raw.isalnum():
        abort(400, "invalid ext")
    ext = ("." + raw) if raw else sniff_ext(data)
    glb = _run(CONVERT_PY, data, ext, "model.glb")
    return Response(glb, mimetype="model/gltf-binary")


def _addr() -> tuple[str, int]:
    host, _, port = os.environ.get("BLENDER_LISTEN", ":9090").rpartition(":")
    return host or "0.0.0.0", int(port or "9090")


if __name__ == "__main__":
    listen_host, listen_port = _addr()
    app.run(host=listen_host, port=listen_port)
