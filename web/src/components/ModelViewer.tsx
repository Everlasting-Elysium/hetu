import "@google/model-viewer";
import type { ModelViewerElement } from "@google/model-viewer";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type DetailedHTMLProps,
  type HTMLAttributes,
  type Ref,
} from "react";
import { fileUrl, modelUrl, thumbUrl } from "../api/client";
import type { Asset } from "../types";
import styles from "./ModelViewer.module.css";

// <model-viewer> is a custom element. Register the JSX types for the string
// attributes we set declaratively; boolean/number settings (camera-controls,
// auto-rotate, ...) are applied imperatively via the ref's reflected properties,
// which sidesteps React's ambiguous handling of boolean attributes on custom
// elements.
type ModelViewerProps = Omit<
  DetailedHTMLProps<HTMLAttributes<HTMLElement>, HTMLElement>,
  "ref"
> & {
  src?: string;
  poster?: string;
  alt?: string;
  ref?: Ref<ModelViewerElement>;
};

declare module "react" {
  namespace JSX {
    interface IntrinsicElements {
      "model-viewer": ModelViewerProps;
    }
  }
}

// DEFAULT_ORBIT / DEFAULT_TARGET frame the initial view; "重置视角" restores them.
const DEFAULT_ORBIT = "0deg 75deg 105%";
const DEFAULT_TARGET = "auto auto auto";

// CLAY_* drives "单色材质" mode: a uniform matte so geometry reads clearly without
// textures. <model-viewer> exposes no true wireframe, so a clay override is the
// honest material-toggle it can render across every model.
const CLAY_COLOR = "#b8b8bd";
const CLAY_ROUGHNESS = 0.85;

// Snapshot of a material's original PBR factors, captured on load so "原始材质"
// restores the exact look after clay mode.
interface Snapshot {
  base: [number, number, number, number];
  metallic: number;
  roughness: number;
}

type Status = "loading" | "ready" | "error";

// ModelViewer renders an interactive 3D preview (#51): orbit/zoom/pan via
// camera-controls, reset-view, an original/clay material toggle, and auto-rotate.
// The model streams from GET /api/dam/assets/{id}/model (glTF/GLB direct, other
// formats converted to GLB server-side). A load failure falls back to the
// thumbnail plus a download link.
export function ModelViewer({ asset }: { asset: Asset }) {
  const ref = useRef<ModelViewerElement>(null);
  const [status, setStatus] = useState<Status>("loading");
  const [clay, setClay] = useState(false);
  const [autoRotate, setAutoRotate] = useState(false);
  const snapshot = useRef<Snapshot[]>([]);
  const label = asset.display_name || asset.name;

  // Enable orbit controls once (reflected boolean property).
  useEffect(() => {
    const el = ref.current;
    if (el) el.cameraControls = true;
  }, []);

  // Snapshot original materials on load; surface load failures for the fallback.
  // load/error are DOM CustomEvents on the element, so they are wired here.
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const onLoad = () => {
      const mats = el.model?.materials ?? [];
      snapshot.current = mats.map((m) => {
        const pbr = m.pbrMetallicRoughness;
        return {
          base: [...pbr.baseColorFactor] as [number, number, number, number],
          metallic: pbr.metallicFactor,
          roughness: pbr.roughnessFactor,
        };
      });
      setStatus("ready");
    };
    const onError = () => setStatus("error");
    el.addEventListener("load", onLoad);
    el.addEventListener("error", onError);
    // Guard the race where the model finished loading before the listener was
    // attached (fast cache hits): reconcile immediately from the live property.
    if (el.loaded) onLoad();
    return () => {
      el.removeEventListener("load", onLoad);
      el.removeEventListener("error", onError);
    };
  }, [asset.id]);

  // Reflect the auto-rotate toggle onto the element.
  useEffect(() => {
    const el = ref.current;
    if (el) el.autoRotate = autoRotate;
  }, [autoRotate]);

  // Apply the material mode whenever it changes or the model becomes ready.
  useEffect(() => {
    const el = ref.current;
    if (!el || status !== "ready" || !el.model) return;
    el.model.materials.forEach((m, i) => {
      const pbr = m.pbrMetallicRoughness;
      if (clay) {
        pbr.setBaseColorFactor(CLAY_COLOR);
        pbr.setMetallicFactor(0);
        pbr.setRoughnessFactor(CLAY_ROUGHNESS);
        return;
      }
      const snap = snapshot.current[i];
      if (!snap) return;
      pbr.setBaseColorFactor(snap.base);
      pbr.setMetallicFactor(snap.metallic);
      pbr.setRoughnessFactor(snap.roughness);
    });
  }, [clay, status]);

  const resetView = useCallback(() => {
    const el = ref.current;
    if (!el) return;
    el.cameraOrbit = DEFAULT_ORBIT;
    el.cameraTarget = DEFAULT_TARGET;
    el.fieldOfView = "auto";
    el.jumpCameraToGoal();
  }, []);

  if (status === "error") {
    return (
      <div className={styles.fallback}>
        {asset.thumb ? (
          <img className={styles.poster} src={thumbUrl(asset.id)} alt={label} />
        ) : null}
        <p className={styles.hint}>无法加载 3D 预览，可下载原文件查看。</p>
        <a
          className="btn btn-primary"
          href={fileUrl(asset.id)}
          target="_blank"
          rel="noreferrer"
          download
        >
          下载文件
        </a>
      </div>
    );
  }

  return (
    <div className={styles.wrap}>
      <model-viewer
        ref={ref}
        className={styles.viewer}
        style={{ width: "100%", height: "100%" }}
        src={modelUrl(asset.id)}
        alt={label}
        // Only set a poster when a thumbnail exists; a missing 3D thumbnail
        // (no Blender sidecar) would otherwise 404 and noise the console.
        {...(asset.thumb ? { poster: thumbUrl(asset.id) } : {})}
      />
      {status === "loading" && (
        <div className={styles.loading}>
          <div className={styles.spinner} />
        </div>
      )}
      <div className={styles.toolbar}>
        <button type="button" onClick={resetView}>
          重置视角
        </button>
        <button
          type="button"
          className={clay ? styles.active : ""}
          data-testid="toggle-material"
          onClick={() => setClay((c) => !c)}
        >
          {clay ? "原始材质" : "单色材质"}
        </button>
        <button
          type="button"
          className={autoRotate ? styles.active : ""}
          onClick={() => setAutoRotate((a) => !a)}
        >
          自动旋转
        </button>
      </div>
    </div>
  );
}
