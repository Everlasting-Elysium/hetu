#!/usr/bin/env python3
"""Blender headless 3D-model thumbnail renderer.

Imports a standard 3D model, frames a camera to its bounds, adds 3-point
lighting, and renders a 512x512 transparent PNG with the EEVEE engine.

Usage:
    blender -b -P render.py -- <input_path> <output_path>

The input extension selects the importer (OBJ/FBX/GLB/GLTF/STL/USD/PLY).
Any failure exits non-zero so the HTTP wrapper can report a render error.
"""
import math
import os
import sys

import bpy
import mathutils

# Import the shared model-import helpers that live alongside this script; Blender
# does not put the script's own directory on sys.path when run with -P.
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from modelio import import_model, parse_args  # noqa: E402

RESOLUTION = 512


def scene_bounds() -> tuple[mathutils.Vector, mathutils.Vector]:
    lo = mathutils.Vector((math.inf, math.inf, math.inf))
    hi = mathutils.Vector((-math.inf, -math.inf, -math.inf))
    found = False
    for obj in bpy.context.scene.objects:
        if obj.type != "MESH":
            continue
        found = True
        for corner in obj.bound_box:
            world = obj.matrix_world @ mathutils.Vector(corner)
            lo = mathutils.Vector(map(min, lo, world))
            hi = mathutils.Vector(map(max, hi, world))
    if not found:
        sys.exit("render.py: no mesh objects imported")
    return lo, hi


def frame_camera(lo: mathutils.Vector, hi: mathutils.Vector) -> None:
    center = (lo + hi) / 2
    radius = max((hi - lo).length / 2, 1e-3)
    cam_data = bpy.data.cameras.new("thumb_cam")
    cam = bpy.data.objects.new("thumb_cam", cam_data)
    bpy.context.scene.collection.objects.link(cam)
    bpy.context.scene.camera = cam
    direction = mathutils.Vector((1.0, -1.0, 0.6)).normalized()
    cam.location = center + direction * radius * 3.0
    look = center - cam.location
    cam.rotation_euler = look.to_track_quat("-Z", "Y").to_euler()


def add_light(name: str, location: mathutils.Vector, energy: float) -> None:
    data = bpy.data.lights.new(name, type="AREA")
    data.energy = energy
    data.size = 5.0
    obj = bpy.data.objects.new(name, data)
    obj.location = location
    bpy.context.scene.collection.objects.link(obj)


def three_point(lo: mathutils.Vector, hi: mathutils.Vector) -> None:
    center = (lo + hi) / 2
    r = max((hi - lo).length, 1.0)
    add_light("key", center + mathutils.Vector((1.0, -1.0, 1.0)) * r, 1000.0)
    add_light("fill", center + mathutils.Vector((-1.0, -1.0, 0.5)) * r, 400.0)
    add_light("rim", center + mathutils.Vector((0.0, 1.0, 1.0)) * r, 600.0)


def configure_render(output_path: str) -> None:
    scene = bpy.context.scene
    for engine in ("BLENDER_EEVEE_NEXT", "BLENDER_EEVEE"):
        try:
            scene.render.engine = engine
            break
        except TypeError:
            continue
    scene.render.resolution_x = RESOLUTION
    scene.render.resolution_y = RESOLUTION
    scene.render.resolution_percentage = 100
    scene.render.film_transparent = True
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.filepath = output_path


def main() -> None:
    input_path, output_path = parse_args()
    bpy.ops.wm.read_factory_settings(use_empty=True)
    import_model(input_path)
    lo, hi = scene_bounds()
    frame_camera(lo, hi)
    three_point(lo, hi)
    configure_render(output_path)
    bpy.ops.render.render(write_still=True)


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as exc:  # noqa: BLE001 - headless: report and fail hard
        print(f"render.py: {exc}", file=sys.stderr)
        sys.exit(1)
