// Honest placeholder for modules the design's navbar shows but the app
// hasn't built yet (spec guardrail: one module fully working before the
// next — cashbacks came first).
export default function Stub({ title }: { title: string }) {
  return (
    <div className="flex flex-col items-center gap-3 pt-24 text-center">
      <span className="flex h-14 w-14 items-center justify-center rounded-2xl border border-brd bg-srf text-2xl">🚧</span>
      <h1 className="text-xl font-extrabold tracking-tight">{title}</h1>
      <p className="max-w-60 text-sm font-medium text-tx3">Модуль в разработке — первым построен «Кешбек».</p>
    </div>
  );
}
