import "@google/model-viewer";
import type { ModelViewerElement } from "@google/model-viewer";
import { useCallback, useEffect, useRef, useState } from "react";
import type { Asset } from "../types";
import { modelUrl, thumbUrl } from "../api/client";
import styles from "./FramePicker.module.css";

// Mirror ModelViewer's initial framing so "重置视角" restores a known pose (#86).
const DEFAULT_ORBIT = "0deg 75deg 105%";
const DEFAULT_TARGET = "auto auto auto";

interface ModelAnglePickerProps {
  asset: Asset;
  initialView?: string;
  onCapture: (blob: Blob, view: string) => void;
  onClose: () => void;
}

// The camera pose serialized onto a board item's `view` field: orbit angles in
// radians + radius in metres, target in metres, field-of-view in degrees — every
// unit model-viewer's setters accept back verbatim to reproduce the angle.
interface ViewPose {
  orbit: { theta: number; phi: number; radius: number };
  target: { x: number; y: number; z: number };
  fov: number;
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null;
}
function num(v: unknown): boolean {
  return typeof v === "number" && Number.isFinite(v);
}
function isViewPose(v: unknown): v is ViewPose {
  if (!isRecord(v) || !isRecord(v.orbit) || !isRecord(v.target)) return false;
  const { orbit, target } = v;
  return (
    num(orbit.theta) && num(orbit.phi) && num(orbit.radius) &&
    num(target.x) && num(target.y) && num(target.z) && num(v.fov)
  );
}

// Restore a stored pose onto the element; a malformed string leaves the default.
function applyView(el: ModelViewerElement, view: string): void {
  let parsed: unknown;
  try {
    parsed = JSON.parse(view);
  } catch {
    return;
  }
  if (!isViewPose(parsed)) return;
  el.cameraOrbit = `${parsed.orbit.theta}rad ${parsed.orbit.phi}rad ${parsed.orbit.radius}m`;
  el.cameraTarget = `${parsed.target.x}m ${parsed.target.y}m ${parsed.target.z}m`;
  el.fieldOfView = `${parsed.fov}deg`;
  el.jumpCameraToGoal();
}

// A modal that pins a 3D camera angle: orbit/zoom/pan the embedded model-viewer
// to the wanted pose, then "捕获此角度" reads the live camera, renders the current
// view to a blob, and hands both the image and the serialized pose back to the
// board. The pose persists on the item so the angle survives a reload.
export function ModelAnglePicker({ asset, initialView, onCapture, onClose }: ModelAnglePickerProps) {
  const ref = useRef<ModelViewerElement>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.cameraControls = true;
    // "neutral" IBL guarantees cross-browser lighting (WebKit renders black
    // without an environment-image), matching ModelViewer.
    el.setAttribute("environment-image", "neutral");
    const onLoad = () => {
      setReady(true);
      if (initialView) applyView(el, initialView);
    };
    el.addEventListener("load", onLoad);
    if (el.loaded) onLoad();
    return () => el.removeEventListener("load", onLoad);
  }, [initialView]);

  const reset = useCallback(() => {
    const el = ref.current;
    if (!el) return;
    el.cameraOrbit = DEFAULT_ORBIT;
    el.cameraTarget = DEFAULT_TARGET;
    el.fieldOfView = "auto";
    el.jumpCameraToGoal();
  }, []);

  const capture = useCallback(async () => {
    const el = ref.current;
    if (!el) return;
    const orbit = el.getCameraOrbit();
    const target = el.getCameraTarget();
    const pose: ViewPose = {
      orbit: { theta: orbit.theta, phi: orbit.phi, radius: orbit.radius },
      target: { x: target.x, y: target.y, z: target.z },
      fov: el.getFieldOfView(),
    };
    const blob = await el.toBlob({ idealAspect: true });
    onCapture(blob, JSON.stringify(pose));
  }, [onCapture]);

  const label = asset.display_name || asset.name;

  return (
    <div className={styles.overlay} onMouseDown={onClose}>
      <div
        className={styles.dialog}
        role="dialog"
        aria-label="选取模型角度"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className={styles.title}>选取角度 · {label}</div>
        <div className={styles.preview}>
          <model-viewer
            ref={ref}
            src={modelUrl(asset.id)}
            alt={label}
            {...(asset.thumb ? { poster: thumbUrl(asset.id) } : {})}
          />
        </div>
        <div className={styles.controls}>
          <button type="button" className={styles.ctrl} onClick={reset}>重置视角</button>
          <span className={styles.hint}>拖拽旋转 · 滚轮缩放 · 右键平移</span>
          <span className={styles.spacer} />
        </div>
        <div className={styles.actions}>
          <button type="button" className="btn btn-ghost" onClick={onClose}>取消</button>
          <button
            type="button"
            className="btn btn-primary"
            disabled={!ready}
            onClick={() => void capture()}
          >
            捕获此角度
          </button>
        </div>
      </div>
    </div>
  );
}
