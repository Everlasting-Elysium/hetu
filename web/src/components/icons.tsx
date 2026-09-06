import type { SVGProps } from "react";
import type { AssetKind } from "../types";

type P = SVGProps<SVGSVGElement>;
const base = (p: P): P => ({
  width: 16,
  height: 16,
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.7,
  strokeLinecap: "round",
  strokeLinejoin: "round",
  ...p,
});

export const IconSearch = (p: P) => (
  <svg {...base(p)}>
    <circle cx="11" cy="11" r="7" />
    <path d="m21 21-4.3-4.3" />
  </svg>
);
export const IconClose = (p: P) => (
  <svg {...base(p)}>
    <path d="M18 6 6 18M6 6l12 12" />
  </svg>
);
export const IconDroplet = (p: P) => (
  <svg {...base(p)}>
    <path d="M12 2.7 6.3 8.4a8 8 0 1 0 11.4 0Z" />
  </svg>
);
export const IconTrash = (p: P) => (
  <svg {...base(p)}>
    <path d="M4 7h16M9 7V5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2m2 0-.8 12a2 2 0 0 1-2 1.8H8.8a2 2 0 0 1-2-1.8L6 7" />
  </svg>
);
export const IconRestore = (p: P) => (
  <svg {...base(p)}>
    <path d="M3 12a9 9 0 1 0 3-6.7L3 8m0-5v5h5" />
  </svg>
);
export const IconFolder = (p: P) => (
  <svg {...base(p)}>
    <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z" />
  </svg>
);
export const IconTag = (p: P) => (
  <svg {...base(p)}>
    <path d="M3 3h7l11 11-7 7L3 10Z" />
    <circle cx="7.5" cy="7.5" r="1.4" fill="currentColor" stroke="none" />
  </svg>
);
export const IconPlus = (p: P) => (
  <svg {...base(p)}>
    <path d="M12 5v14M5 12h14" />
  </svg>
);
export const IconStar = (p: P) => (
  <svg {...base(p)} strokeWidth={1.4}>
    <path d="M12 3.6l2.6 5.3 5.8.8-4.2 4.1 1 5.8L12 17l-5.2 2.7 1-5.8-4.2-4.1 5.8-.8Z" />
  </svg>
);
export const IconGrid = (p: P) => (
  <svg {...base(p)}>
    <rect x="3" y="3" width="7" height="7" rx="1" />
    <rect x="14" y="3" width="7" height="7" rx="1" />
    <rect x="3" y="14" width="7" height="7" rx="1" />
    <rect x="14" y="14" width="7" height="7" rx="1" />
  </svg>
);
export const IconBoard = (p: P) => (
  <svg {...base(p)}>
    <rect x="3" y="3" width="18" height="18" rx="2" />
    <rect x="7" y="7" width="4" height="4" rx="0.5" />
    <rect x="13" y="12" width="5" height="5" rx="0.5" />
  </svg>
);
export const IconArrowLeft = (p: P) => (
  <svg {...base(p)}>
    <path d="M19 12H5m0 0 6-6m-6 6 6 6" />
  </svg>
);
export const IconPencil = (p: P) => (
  <svg {...base(p)}>
    <path d="M4 20h4L18.5 9.5a2 2 0 0 0-2.8-2.8L5 17.2Zm10.5-13 2.8 2.8" />
  </svg>
);
export const IconAlert = (p: P) => (
  <svg {...base(p)}>
    <path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z" />
    <path d="M12 9v4" />
    <path d="M12 17h.01" />
  </svg>
);

// File-type placeholder glyphs, keyed by asset kind, for missing thumbnails.
const KIND_PATHS: Record<AssetKind, string> = {
  image: "M4 5h16v14H4Zm3 9 3-3 4 4 3-2 3 3M8.5 9.5h.01",
  video: "M4 5h16v14H4Zm6 3 6 4-6 4Z",
  audio: "M9 18V6l10-2v11M9 15a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0Zm10-2a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0Z",
  model: "M12 3 3 8v8l9 5 9-5V8Zm0 0v18M3 8l9 5 9-5",
  document: "M6 3h8l4 4v14H6Zm8 0v4h4M9 13h6M9 17h6",
  other: "M6 3h8l4 4v14H6Zm8 0v4h4",
};

export const KindIcon = ({ kind, ...p }: P & { kind: AssetKind }) => (
  <svg {...base({ width: 40, height: 40, strokeWidth: 1.3, ...p })}>
    <path d={KIND_PATHS[kind] ?? KIND_PATHS.other} />
  </svg>
);
