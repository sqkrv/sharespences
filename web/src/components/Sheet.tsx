import { useEffect, useRef, useState, type ReactNode } from "react";
import { Card } from "./ui";

// Bottom sheet (W-05): the redesign's пикер месяца and offer cards. Backdrop
// tap closes; the card stops propagation — the Partners offer-sheet idiom,
// extracted. Becomes a centered modal on sm:.
//
// The grab handle is a real control: dragging the top of the sheet down
// dismisses it (a handle that promises dragging and ignores it reads as
// broken — feedback 2026-08-25, and on small screens the sheet can cover
// the backdrop entirely, leaving no other way out). While the sheet is
// open the page behind must not scroll; overflow:hidden leaks on iOS, so
// the body is pinned with position:fixed and restored on close.
//
// data-sid sits on the Card, not the fixed backdrop (dev-mode rule), and the
// card clips its own overflow, so the tag opts into the inside placement.
export function Sheet({
  onClose,
  title,
  sid,
  children,
}: {
  onClose: () => void;
  title?: string;
  sid?: string;
  children: ReactNode;
}) {
  const [dragY, setDragY] = useState(0);
  const dragFrom = useRef<number | null>(null);
  // The release decision reads the ref, not the state: a fast flick delivers
  // down/move/up inside one frame, before any re-render refreshes closures.
  const dragNow = useRef(0);
  const setDrag = (y: number) => {
    dragNow.current = y;
    setDragY(y);
  };

  useEffect(() => {
    const b = document.body.style;
    const y = window.scrollY;
    const prev = { position: b.position, top: b.top, left: b.left, right: b.right, overflow: b.overflow };
    b.position = "fixed";
    b.top = `-${y}px`;
    b.left = "0";
    b.right = "0";
    b.overflow = "hidden";
    return () => {
      b.position = prev.position;
      b.top = prev.top;
      b.left = prev.left;
      b.right = prev.right;
      b.overflow = prev.overflow;
      window.scrollTo(0, y);
    };
  }, []);

  const release = () => {
    const passed = dragNow.current > 70;
    dragFrom.current = null;
    if (passed) onClose();
    else setDrag(0);
  };

  return (
    <div
      className="fixed inset-0 z-40 flex items-end justify-center bg-black/45 sm:items-center sm:p-4"
      onClick={onClose}
    >
      <Card
        className="max-h-[88vh] w-full max-w-md overflow-y-auto overscroll-contain rounded-t-[26px] rounded-b-none px-4 pt-0 pb-[max(env(safe-area-inset-bottom),1rem)] sm:rounded-2xl"
        onClick={(e: React.MouseEvent) => e.stopPropagation()}
        data-sid={sid}
        data-sid-inside={sid ? "" : undefined}
        style={{
          transform: dragY ? `translateY(${dragY}px)` : undefined,
          transition: dragFrom.current == null ? "transform .18s ease" : "none",
        }}
      >
        {/* The drag zone: handle plus title row, wide on purpose. */}
        <div
          className="-mx-4 cursor-grab touch-none px-4 pt-2 select-none"
          onPointerDown={(e) => {
            dragFrom.current = e.clientY;
            try {
              e.currentTarget.setPointerCapture(e.pointerId);
            } catch {
              // Untracked pointer (synthetic events) — the move/up targets
              // are ours anyway.
            }
          }}
          onPointerMove={(e) => {
            if (dragFrom.current == null) return;
            setDrag(Math.max(0, e.clientY - dragFrom.current));
          }}
          onPointerUp={release}
          onPointerCancel={release}
        >
          <span className="mx-auto mb-2 block h-1 w-9 rounded-full bg-inset" />
          {title && <p className="mb-2 px-0.5 text-[15px] font-extrabold tracking-[-.02em]">{title}</p>}
        </div>
        {children}
      </Card>
    </div>
  );
}
