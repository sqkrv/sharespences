// Fails the build on a high/critical npm advisory in what actually ships.
//
// Reads `npm audit --omit=dev --json` on stdin. Build tooling is deliberately
// out of scope: vite, workbox and openapi-typescript process our own generated
// files, never attacker input, so an advisory there is not a vulnerability in
// this product — and a gate that cries wolf about them gets ignored.
//
// Advisories judged not-applicable are listed in ACCEPTED_ADVISORIES below,
// each with the reason. That keeps an acceptance a reviewable line rather than
// a lowered threshold, and the staleness check makes sure a forgotten entry
// cannot silently cover a future advisory in the same package.
//
// Usage: npm audit --omit=dev --json | node scripts/audit-gate.mjs

const ACCEPTED_ADVISORIES = {
  // React Router RSC-mode CSRF bypass. This SPA mounts <BrowserRouter> with
  // declarative <Routes>/<Route> only — no data router, no loaders, no
  // actions, no RSC — so the affected pipeline does not exist here. npm's
  // suggested fix is a DOWNGRADE to 7.11.0; the forward fix is react-router 8,
  // which is a major migration. Drop this entry when that lands.
  "GHSA-qwww-vcr4-c8h2": "react-router RSC mode is not used (BrowserRouter only)",
};

const raw = await new Promise((resolve, reject) => {
  let buf = "";
  process.stdin.setEncoding("utf8");
  process.stdin.on("data", (chunk) => (buf += chunk));
  process.stdin.on("end", () => resolve(buf));
  process.stdin.on("error", reject);
});

let report;
try {
  report = JSON.parse(raw);
} catch {
  console.error("audit-gate: could not parse npm audit --json output");
  process.exit(2);
}

// Only object-form `via` entries carry an advisory; string entries are just
// links to the parent package that pulled the vulnerable one in.
const found = new Map();
for (const vuln of Object.values(report.vulnerabilities ?? {})) {
  for (const via of vuln.via ?? []) {
    if (typeof via !== "object" || !via.url) continue;
    if (via.severity !== "high" && via.severity !== "critical") continue;
    found.set(via.url.split("/").pop(), `${via.severity} · ${via.name} — ${via.title}`);
  }
}

const unaccepted = [...found].filter(([id]) => !(id in ACCEPTED_ADVISORIES));
if (unaccepted.length) {
  console.error("Unaccepted high/critical advisories in shipped dependencies:\n");
  for (const [id, what] of unaccepted) console.error(`  ${id}  ${what}`);
  console.error("\nUpgrade it, or add the id to ACCEPTED_ADVISORIES with the reason it does not apply.");
  process.exit(1);
}

const stale = Object.keys(ACCEPTED_ADVISORIES).filter((id) => !found.has(id));
if (stale.length) {
  console.error(`Accepted advisories that no longer appear: ${stale.join(", ")}`);
  console.error("Remove them — a stale acceptance would silently cover the next advisory in that package.");
  process.exit(1);
}

const n = Object.keys(ACCEPTED_ADVISORIES).length;
console.log(`npm audit: no unaccepted high/critical advisories in shipped dependencies (${n} accepted)`);
