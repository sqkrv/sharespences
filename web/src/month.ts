import { midMonthISO } from "./lib";

// The viewed month, shared by the feed (CB-01) and «Банки» (CB-09): both
// screens carry the same month chip, and hopping between them must not snap
// a backfilling user to the current month. Module state, not storage — the
// choice is a view preference for one visit, and privacy.html §3.2
// enumerates every persisted key, so it deliberately does not earn one.
let viewed: string | null = null;

export function viewedMonth(): string {
  if (viewed) return viewed;
  const now = new Date();
  return midMonthISO(now.getFullYear(), now.getMonth());
}

export function rememberMonth(iso: string) {
  viewed = iso;
}
