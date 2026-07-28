// A ring that tells the truth about a long job: the solid arc is work
// actually FINISHED (screenshots read), and the sweeping arc on top is the
// one in flight. Recognition has no honest sub-progress inside a single
// screenshot — the model returns once, minutes later — so the in-flight
// part animates rather than pretending to fill. The caption next to it
// carries the phase.
export default function ProgressRing({
  done,
  total,
  active,
  size = 46,
  label,
}: {
  done: number;
  total: number;
  active?: boolean; // something is in flight → sweep the next segment
  size?: number;
  label?: string; // centre text; defaults to done/total
}) {
  const stroke = 4;
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const safeTotal = Math.max(total, 1);
  const filled = Math.min(Math.max(done, 0), safeTotal) / safeTotal;
  // The sweeping arc covers one screenshot's worth of the ring, so it
  // reads as «this one is being worked on», not as extra progress.
  const sweep = active ? c / safeTotal : 0;

  return (
    <div className="relative flex-none" style={{ width: size, height: size }} role="img" aria-label={`${done} из ${total}`}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="-rotate-90">
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--t-inset)" strokeWidth={stroke} />
        {sweep > 0 && (
          <circle
            cx={size / 2}
            cy={size / 2}
            r={r}
            fill="none"
            stroke="var(--t-acc)"
            strokeWidth={stroke}
            strokeLinecap="round"
            strokeDasharray={`${sweep} ${c - sweep}`}
            strokeDashoffset={-filled * c}
            className="origin-center animate-[ringsweep_2.4s_linear_infinite] opacity-70"
          />
        )}
        {filled > 0 && (
          <circle
            cx={size / 2}
            cy={size / 2}
            r={r}
            fill="none"
            stroke="var(--t-mint)"
            strokeWidth={stroke}
            strokeLinecap="round"
            strokeDasharray={`${filled * c} ${c}`}
            className="transition-[stroke-dasharray] duration-500"
          />
        )}
      </svg>
      <span className="absolute inset-0 flex items-center justify-center text-[11px] font-extrabold tabular-nums">
        {label ?? `${done}/${total}`}
      </span>
    </div>
  );
}
