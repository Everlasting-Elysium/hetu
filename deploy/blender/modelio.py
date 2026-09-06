#!/usr/bin/env python3
"""Shared Blender model-import helpers for the headless entry points.

render.py (thumbnails, #12) and convert.py (GLB conversion for the web viewer,
#51) both import a standard 3D model the same way. Centralising the
extension -> importer table and CLI parsing here keeps the two scripts from
drifting. Imported from within Blender (`blender -b -P <script>`), so `bpy` is
always available.
"""
import sys

import bpy

# extension -> Blender import operator (Blender 4.x operators).
IMPORTERS = {
    ".obj": lambda p: bpy.ops.wm.obj_import(filepath=p),
    ".fbx": lambda p: bpy.ops.import_scene.fbx(filepath=p),
    ".glb": lambda p: bpy.ops.import_scene.gltf(filepath=p),
    ".gltf": lambda p: bpy.ops.import_scene.gltf(filepath=p),
    ".stl": lambda p: bpy.ops.wm.stl_import(filepath=p),
    ".usd": lambda p: bpy.ops.wm.usd_import(filepath=p),
    ".usdz": lambda p: bpy.ops.wm.usd_import(filepath=p),
    ".ply": lambda p: bpy.ops.wm.ply_import(filepath=p),
}


def parse_args() -> tuple[str, str]:
    """Return (input_path, output_path) from argv after the '--' separator."""
    argv = sys.argv
    if "--" not in argv:
        sys.exit("modelio: missing '--' argument separator")
    rest = argv[argv.index("--") + 1:]
    if len(rest) != 2:
        sys.exit("modelio: expected <input_path> <output_path>")
    return rest[0], rest[1]


def import_model(path: str) -> None:
    """Import path into the current scene, picking the importer by extension."""
    ext = path[path.rfind("."):].lower()
    importer = IMPORTERS.get(ext)
    if importer is None:
        sys.exit(f"modelio: unsupported format {ext!r}")
    importer(path)
