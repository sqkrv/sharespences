import { useCallback, useEffect, useRef, useState } from "react";

import { attachmentURL } from "../api/client";

// W-04 — full-screen screenshot viewer.
//
// Bank screenshots are read, not glanced at: opening one means zooming into
// «Кешбэк до 2 000 ₽» or checking which section a row sat under. The
// thumbnails used to link to the raw file with target="_blank", which looked
// like the app had given up — but it did hand over the browser's own
// pinch-zoom, so a viewer that merely fits the image to the screen would have
// been a downgrade. Hence the gestures here are real: pinch, double-tap, pan,
// and a swipe filmstrip across the period's shots.
//
// Hand-rolled on pointer events rather than a lightbox dependency (~30 KB for
// one screen's worth of behaviour), and it persists nothing — the privacy
// policy enumerates every localStorage key (§3.2), so a viewer that
// remembered its zoom would be a document edit.

const MAX_SCALE = 6;
const DOUBLE_TAP_SCALE = 2.5;
// A tap is a pointer that barely moved and let go quickly; anything else is a
// drag. Fingers are imprecise, so the double-tap window is generous in space.
const TAP_SLOP = 10;
const TAP_MS = 300;
const DOUBLE_TAP_MS = 300;
const DOUBLE_TAP_SLOP = 40;
// Past a quarter of the viewport the swipe pages; otherwise it snaps back.
const PAGE_FRACTION = 0.25;
const CLOSE_DRAG = 110;

type Zoom = { scale: number; x: number; y: number };
const NO_ZOOM: Zoom = { scale: 1, x: 0, y: 0 };

type Gesture = {
  kind: "pan" | "pinch" | "swipe";
  zoom: Zoom; // the transform this gesture started from
  x: number;
  y: number; // first pointer down, or the pinch midpoint
  dist: number; // pinch only
  at: number;
  moved: boolean;
  axis: "x" | "y" | null; // locked once a swipe commits to a direction
  dx: number;
  dy: number;
};

function clamp(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v));
}

export function Lightbox({
  ids,
  startIndex = 0,
  alt = "скриншот",
  onClose,
}: {
  ids: string[];
  startIndex?: number;
  alt?: string;
  onClose: () => void;
}) {
  const [i, setI] = useState(() =>
    clamp(startIndex, 0, Math.max(0, ids.length - 1)),
  );
  const [zoom, setZoom] = useState<Zoom>(NO_ZOOM);
  const [drag, setDrag] = useState({ x: 0, y: 0, active: false });

  const stageRef = useRef<HTMLDivElement>(null);
  const imgRef = useRef<HTMLImageElement>(null);
  const pointers = useRef(new Map<number, { x: number; y: number }>());
  const gesture = useRef<Gesture | null>(null);
  const lastTap = useRef({ at: 0, x: 0, y: 0 });

  const go = useCallback(
    (next: number) => {
      setI(clamp(next, 0, ids.length - 1));
      setZoom(NO_ZOOM); // a fresh screenshot starts fitted, never mid-zoom
    },
    [ids.length],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      else if (e.key === "ArrowLeft") go(i - 1);
      else if (e.key === "ArrowRight") go(i + 1);
    };
    window.addEventListener("keydown", onKey);
    // The page behind must not scroll under the overlay — on iOS a rubber-band
    // there is what makes a viewer feel broken.
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
    };
  }, [i, go, onClose]);

  // Keep the image within the stage: at scale 1 it is centred and immovable,
  // and it can never be dragged past its own edge.
  const clampPan = (z: Zoom): Zoom => {
    const img = imgRef.current;
    const stage = stageRef.current;
    if (!img || !stage) return z;
    const maxX = Math.max(
      0,
      (img.offsetWidth * z.scale - stage.clientWidth) / 2,
    );
    const maxY = Math.max(
      0,
      (img.offsetHeight * z.scale - stage.clientHeight) / 2,
    );
    return {
      scale: z.scale,
      x: clamp(z.x, -maxX, maxX),
      y: clamp(z.y, -maxY, maxY),
    };
  };

  // Scale about a point, keeping the content under it pinned — the difference
  // between a zoom that follows your fingers and one that jumps to the centre.
  const zoomAbout = (
    from: Zoom,
    scale: number,
    at: { x: number; y: number },
  ): Zoom => {
    const stage = stageRef.current;
    if (!stage) return from;
    const s = clamp(scale, 1, MAX_SCALE);
    if (s === 1) return NO_ZOOM;
    const cx = stage.clientWidth / 2;
    const cy = stage.clientHeight / 2;
    const px = (at.x - cx - from.x) / from.scale;
    const py = (at.y - cy - from.y) / from.scale;
    return clampPan({ scale: s, x: at.x - cx - px * s, y: at.y - cy - py * s });
  };

  const stagePoint = (e: React.PointerEvent) => {
    const r = stageRef.current?.getBoundingClientRect();
    return { x: e.clientX - (r?.left ?? 0), y: e.clientY - (r?.top ?? 0) };
  };

  const onPointerDown = (e: React.PointerEvent) => {
    (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
    const p = stagePoint(e);
    pointers.current.set(e.pointerId, p);
    const pts = [...pointers.current.values()];
    if (pts.length === 2) {
      const [a, b] = pts;
      gesture.current = {
        kind: "pinch",
        zoom,
        x: (a.x + b.x) / 2,
        y: (a.y + b.y) / 2,
        dist: Math.hypot(a.x - b.x, a.y - b.y),
        at: Date.now(),
        moved: true,
        axis: null,
        dx: 0,
        dy: 0,
      };
    } else if (pts.length === 1) {
      gesture.current = {
        kind: zoom.scale > 1 ? "pan" : "swipe",
        zoom,
        x: p.x,
        y: p.y,
        dist: 0,
        at: Date.now(),
        moved: false,
        axis: null,
        dx: 0,
        dy: 0,
      };
    }
  };

  const onPointerMove = (e: React.PointerEvent) => {
    const g = gesture.current;
    if (!g || !pointers.current.has(e.pointerId)) return;
    const p = stagePoint(e);
    pointers.current.set(e.pointerId, p);

    if (g.kind === "pinch") {
      const pts = [...pointers.current.values()];
      if (pts.length < 2) return;
      const [a, b] = pts;
      const dist = Math.hypot(a.x - b.x, a.y - b.y);
      const mid = { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
      setZoom(zoomAbout(g.zoom, (g.zoom.scale * dist) / (g.dist || dist), mid));
      return;
    }

    g.dx = p.x - g.x;
    g.dy = p.y - g.y;
    if (!g.moved && Math.hypot(g.dx, g.dy) > TAP_SLOP) g.moved = true;

    if (g.kind === "pan") {
      setZoom(
        clampPan({
          scale: g.zoom.scale,
          x: g.zoom.x + g.dx,
          y: g.zoom.y + g.dy,
        }),
      );
      return;
    }
    // Swipe: lock to one axis on the first real movement, so a horizontal
    // page does not also drag the sheet towards dismissal.
    if (!g.axis && g.moved)
      g.axis = Math.abs(g.dx) > Math.abs(g.dy) ? "x" : "y";
    setDrag({
      x: g.axis === "x" ? g.dx : 0,
      y: g.axis === "y" ? Math.max(0, g.dy) : 0,
      active: true,
    });
  };

  const handleTap = (g: Gesture) => {
    const now = Date.now();
    const isDouble =
      now - lastTap.current.at < DOUBLE_TAP_MS &&
      Math.hypot(g.x - lastTap.current.x, g.y - lastTap.current.y) <
        DOUBLE_TAP_SLOP;
    lastTap.current = { at: isDouble ? 0 : now, x: g.x, y: g.y }; // a third tap starts a new pair
    if (isDouble) {
      setZoom(
        zoom.scale > 1
          ? NO_ZOOM
          : zoomAbout(NO_ZOOM, DOUBLE_TAP_SCALE, { x: g.x, y: g.y }),
      );
      return;
    }
    // A single tap on the letterbox — beside the image, not on it — closes.
    // Tapping the image itself does nothing, which is what leaves double-tap
    // unambiguous and spares the close a 300 ms deferral.
    const img = imgRef.current;
    const stage = stageRef.current;
    if (!img || !stage) return;
    const ir = img.getBoundingClientRect();
    const sr = stage.getBoundingClientRect();
    const x = g.x + sr.left;
    const y = g.y + sr.top;
    if (x < ir.left || x > ir.right || y < ir.top || y > ir.bottom) onClose();
  };

  const endGesture = (e: React.PointerEvent) => {
    const g = gesture.current;
    pointers.current.delete(e.pointerId);
    if (!g) return;
    if (pointers.current.size > 0) {
      // A finger lifted out of a pinch: end the gesture rather than
      // reinterpreting the remaining pointer mid-flight.
      gesture.current = null;
      setDrag({ x: 0, y: 0, active: false });
      return;
    }
    gesture.current = null;
    if (g.kind === "swipe") {
      const width = stageRef.current?.clientWidth ?? 1;
      if (!g.moved && Date.now() - g.at < TAP_MS) {
        handleTap(g);
      } else if (g.axis === "y" && g.dy > CLOSE_DRAG) {
        onClose();
        return;
      } else if (g.axis === "x" && Math.abs(g.dx) > width * PAGE_FRACTION) {
        go(i + (g.dx < 0 ? 1 : -1));
      }
    }
    setDrag({ x: 0, y: 0, active: false });
  };

  const many = ids.length > 1;
  // Dragging towards dismissal fades the backdrop, so the gesture reads as
  // «putting it back» rather than as a stuck screen.
  const dim = drag.y > 0 ? clamp(1 - drag.y / (CLOSE_DRAG * 2.4), 0.3, 1) : 1;

  return (
    <div
      className="fixed inset-0 z-50 flex flex-col"
      style={{ backgroundColor: `rgba(0,0,0,${0.93 * dim})` }}
      role="dialog"
      aria-modal="true"
      aria-label="Просмотр скриншота"
    >
      <div className="flex flex-none items-center justify-between px-3 pt-[max(env(safe-area-inset-top),10px)] pb-2">
        <span className="text-[12px] font-semibold text-white/60">
          {many ? `${i + 1} из ${ids.length}` : ""}
        </span>
        <button
          type="button"
          onClick={onClose}
          aria-label="Закрыть"
          className="flex h-9 w-9 items-center justify-center rounded-full bg-white/10 text-[15px] font-bold text-white/80"
        >
          ✕
        </button>
      </div>

      <div
        ref={stageRef}
        data-sid="W-04"
        data-sid-inside
        className="relative flex-1 touch-none overflow-hidden"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={endGesture}
        onPointerCancel={endGesture}
      >
        <div
          className="flex h-full w-full"
          style={{
            transform: `translate3d(calc(${-i * 100}% + ${drag.x}px), ${drag.y}px, 0)`,
            transition: drag.active
              ? "none"
              : "transform 220ms cubic-bezier(.22,.61,.36,1)",
          }}
        >
          {ids.map((id, n) => (
            <div
              key={id}
              className="flex h-full w-full flex-none items-center justify-center p-2"
            >
              <img
                ref={n === i ? imgRef : undefined}
                src={attachmentURL(id)}
                alt={alt}
                draggable={false}
                className="max-h-full max-w-full origin-center object-contain select-none"
                style={
                  n === i
                    ? {
                        transform: `translate(${zoom.x}px, ${zoom.y}px) scale(${zoom.scale})`,
                        transition: gesture.current
                          ? "none"
                          : "transform 180ms ease-out",
                      }
                    : undefined
                }
              />
            </div>
          ))}
        </div>

        {many && (
          <>
            <button
              type="button"
              onClick={() => go(i - 1)}
              disabled={i === 0}
              aria-label="Предыдущий скриншот"
              className="absolute top-1/2 left-2 hidden h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 text-white/80 disabled:opacity-25 sm:flex"
            >
              ‹
            </button>
            <button
              type="button"
              onClick={() => go(i + 1)}
              disabled={i === ids.length - 1}
              aria-label="Следующий скриншот"
              className="absolute top-1/2 right-2 hidden h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 text-white/80 disabled:opacity-25 sm:flex"
            >
              ›
            </button>
          </>
        )}
      </div>

      {many && (
        <div className="flex flex-none items-center justify-center gap-1.5 pt-2 pb-[max(env(safe-area-inset-bottom),12px)]">
          {ids.map((id, n) => (
            <button
              key={id}
              type="button"
              onClick={() => go(n)}
              aria-label={`Скриншот ${n + 1}`}
              className={`h-1.5 rounded-full transition-all ${n === i ? "w-4 bg-white/80" : "w-1.5 bg-white/30"}`}
            />
          ))}
        </div>
      )}
    </div>
  );
}
