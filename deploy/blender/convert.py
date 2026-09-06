#!/usr/bin/env python3
"""Blender headless 3D-model -> GLB converter for the web viewer (#51).

Imports a non-web-friendly model (OBJ/FBX/STL/USD/PLY) and re-exports it as one
self-contained GLB that <model-viewer> can load directly: GLB embeds geometry,
materials, and textures in a single binary, so the browser needs no sidecar
files. glTF/GLB are served as-is by the backend and never reach this script.

Usage:
    blender -b -P convert.py -- <input_path> <output_path.glb>

Any failure exits non-zero so the HTTP wrapper can report a conversion error.
"""
import os
import sys

import bpy

# Import the shared model-import helpers that live alongside this script; Blender
# does not put the script's own directory on sys.path when run with -P.
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from modelio import import_model, parse_args  # noqa: E402


def main() -> None:
    input_path, output_path = parse_args()
    bpy.ops.wm.read_factory_settings(use_empty=True)
    import_model(input_path)
    bpy.ops.export_scene.gltf(filepath=output_path, export_format="GLB")


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as exc:  # noqa: BLE001 - headless: report and fail hard
        print(f"convert.py: {exc}", file=sys.stderr)
        sys.exit(1)
