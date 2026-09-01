# Bank logos

Drop a file here named `<slug>.svg` (or `.png`) and the badge picks it up on the
next build — no code change. Missing file ⇒ the two-letter fallback («АБ», «ОЗ»)
renders instead, so a partial set is fine.

| slug | bank | brand color |
|---|---|---|
| `alfabank` | Альфа-Банк | `#EF3124` |
| `vtb` | ВТБ | `#0A2896` |
| `ozon` | Озон Банк | `#005BFF` |
| `yandex-pay` | Яндекс Пэй | `#FC3F1D` |
| `gazprombank` | Газпромбанк | `#10069F` |
| `mkb` | МКБ | `#E31E24` |
| `sber` | СберБанк | `#21A038` |
| `tbank` | Т-Банк | `#FFDD2D` |
| `sovkom` | Совкомбанк | `#213A8B` |
| `otp` | ОТП Банк | `#C3FF0B` |
| `mtsmoney` | МТС Деньги | `#F21630` ⚠️ midpoint of a gradient |
| `ubrr` | УБРиР | `#CC163F` |
| `pskb` | Примсоцбанк | `#BEA980` |
| `sinara` | Банк Синара | `#E40134` |

**Where the brand color comes from.** Read it off the logo's own background —
most files carry it on a `class="bg-logo"` element, the rest on a shape covering
the whole viewBox. That is the right source rather than merely the convenient
one: the color exists to tint the two-letter chip that *stands in for that
logo*, so matching the icon's background is what makes the fallback read as a
placeholder for the mark rather than an unrelated square.

⚠️ It is **not** the same thing as the bank's marketing brand color, and several
banks differ on the two — see the discrepancy list in
`docs/knowledge/seed-reconciliation.md`. Do not sample a website for this; a
homepage's most-frequent hex is a different question with a coincidentally
similar answer.

The slug ↔ bank-name mapping lives in `BANK_SLUG` (`web/src/components/ui.tsx`)
and is keyed by the seeded `bank.name`; add banks there when the seed grows.

## What the file must be

The badge renders the logo **as-is, with no plaque behind it** (owner decision
2026-07-27), at **22–33 px** on a dark-first surface. That makes the file
requirements narrow:

- **The app-icon / avatar version of the mark**, i.e. self-contained: the mark
  on its own brand background where the brand draws it that way (Т-Банк's yellow
  shield, Озон's blue tile). A bare dark glyph meant for white paper disappears
  on the dark theme — if the brand has no self-contained variant, supply the
  mark on its brand-colored square.
- **Symbol only, never the wordmark** — at 22 px a wordmark is a smudge.
- **Square artwork** (1:1). Non-square is letterboxed, not cropped, so it will
  look smaller than its neighbours.
- SVG: a `viewBox` and **no `width`/`height`** — an intrinsic size is what makes a file refuse to
  scale. A 56×56 file carrying both keeps rendering at 56 px inside a larger frame, so the mark ends
  up stranded in the corner of a mostly-empty tile in Finder and in any preview that does not force
  a size. The files that behave (`alfabank`, `vtb`, `ubrr`, `sinara`) carry a `viewBox` alone.
  - ⚠️ **`svgo` strips the `viewBox` by default.** `removeViewBox` is in `preset-default` and fires
    whenever the `viewBox` matches `width`/`height`, turning an optimisation pass into a file that
    cannot scale at all. Pass `--disable=removeViewBox`, then check the root tag by eye.
- ⚠️ **No `<mask>`, and specifically never `mask-type: alpha`.** Quick Look ignores that property
  and falls back to the default `mask-type: luminance`, where a mask path filled `#000` has zero
  luminance and hides everything inside it — leaving just the background rect. The symptom is a file
  that renders correctly in the app and as a **plain white tile in Finder**, which reads as a broken
  export rather than a rendering gap. Figma emits exactly this shape when a frame is set to clip
  contents. Use `<clipPath>` if a clip is genuinely needed; more often the mask is trimming nothing
  and can go. (Hit on `gazprombank.svg`, 2026-09-01.)
- No external references (`<image href="http…">`, web fonts),
  text converted to outlines. PNG fallback: **≥ 256×256**, transparent outside
  the mark.
- Optical sizing: the artwork should fill its square the way an app icon does.
  Large transparent margins make one bank look shrunken next to the others.

## Attribution

Every file added here needs a row in the repo's `ACKNOWLEDGEMENTS.md` — source
URL and rights holder. These are third-party trademarks; the repo is public.
