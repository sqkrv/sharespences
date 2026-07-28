import { useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { clearJob, useActiveRecognition, useRecognitionPoll } from "../recognition";

// The answer to «I uploaded screenshots and left the screen — how do I know
// it's done?». The job id lives in ?job=…, which disappears on the first
// navigation, so without this chip a running recognition is only reachable
// by pressing Back. It also keeps the poll alive app-wide, which is what
// captures the finished draft locally before the server's 30-minute window
// expires. Web Push is deliberately out (docs/specs/pwa.md), so «the app
// tells you the moment it is open» is the honest ceiling.
export default function RecognitionChip() {
  const job = useActiveRecognition();
  const poll = useRecognitionPoll(job);
  const navigate = useNavigate();
  const location = useLocation();
  const [params] = useSearchParams();

  if (job == null) return null;
  // Redundant while you are already looking at that job's review screen.
  if (location.pathname === "/periods/new" && params.get("job") === job.id) return null;

  const ready = job.state.rows != null;
  const failed = !ready && (poll.isError || poll.data?.status === "failed");
  const done = poll.data?.done ?? 0;
  const total = poll.data?.total ?? job.state.attachmentIDs.length;

  const tone = ready
    ? "border-mint/40 text-mint"
    : failed
      ? "border-warn/40 text-warn"
      : "border-brd2 text-tx2";
  const label = ready
    ? "распознано — проверь и сохрани"
    : failed
      ? "распознать не удалось"
      : `распознаём скриншоты · ${done} из ${total}`;

  return (
    // pointer-events-none on the full-width strip, auto on the pill — the
    // band must not swallow taps on the content behind it (as OfflineChip).
    <div className="pointer-events-none fixed inset-x-0 bottom-[136px] z-40 flex justify-center px-4">
      <div className={`pointer-events-auto flex items-center gap-1 rounded-2xl border ${tone} bg-srf/95 py-1.5 pl-3 pr-1.5 shadow-[0_14px_40px_-12px_rgba(0,0,0,.55)] backdrop-blur`}>
        <button
          type="button"
          onClick={() => navigate(`/periods/new?job=${job.id}`)}
          className="flex items-center gap-2 text-[12px] font-bold"
        >
          {!ready && !failed && <span className="h-1.5 w-1.5 flex-none animate-pulse rounded-full bg-acc" />}
          <span>{label}</span>
        </button>
        {/* Only a settled job may be dismissed — cancelling a running one
            here would leave it running on the server anyway. */}
        {(ready || failed) && (
          <button type="button" aria-label="убрать" className="px-1.5 text-[13px] text-tx4" onClick={() => clearJob(job.id)}>
            ✕
          </button>
        )}
      </div>
    </div>
  );
}
