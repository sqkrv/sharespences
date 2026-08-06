import type { ReactNode } from "react";
import { Card } from "./ui";

// Bottom sheet (W-05): the redesign's пикер месяца and offer cards. Backdrop
// tap closes; the card stops propagation — the Partners offer-sheet idiom,
// extracted. Becomes a centered modal on sm:. The grab handle is decorative
// (no drag gesture — tap the backdrop or the caller's own control).
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
  return (
    <div
      className="fixed inset-0 z-40 flex items-end justify-center bg-black/45 sm:items-center sm:p-4"
      onClick={onClose}
    >
      <Card
        className="max-h-[88vh] w-full max-w-md overflow-y-auto rounded-t-[26px] rounded-b-none px-4 pt-2 pb-[max(env(safe-area-inset-bottom),1rem)] sm:rounded-2xl"
        onClick={(e: React.MouseEvent) => e.stopPropagation()}
        data-sid={sid}
        data-sid-inside={sid ? "" : undefined}
      >
        <span className="mx-auto mb-2 block h-1 w-9 rounded-full bg-inset" />
        {title && <p className="mb-2 px-0.5 text-[15px] font-extrabold tracking-[-.02em]">{title}</p>}
        {children}
      </Card>
    </div>
  );
}
