// Session-only «Недавнее» for the search screen (redesign 2i). Deliberately
// NOT localStorage: privacy policy §3.2 enumerates every stored key as an
// exhaustive published claim, and search history is not worth a policy
// edition — the list dies with the page, which is honest enough for a
// convenience section.
export type RecentEntry = { label: string; sub?: string; to: string };

let RECENT: RecentEntry[] = [];

export function pushRecent(e: RecentEntry) {
  RECENT = [e, ...RECENT.filter((r) => r.to !== e.to)].slice(0, 6);
}

export function recentEntries(): RecentEntry[] {
  return RECENT;
}
