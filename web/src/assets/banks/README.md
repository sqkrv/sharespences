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
| `sovkom` | Совкомбанк | — |
| `otp` | ОТП Банк | — |
| `mtsmoney` | МТС Деньги | — |
| `ubrr` | УБРиР | `#CC163F` ⚠️ sampled |
| `pskb` | Примсоцбанк | `#008F4C` ⚠️ sampled |
| `sinara` | Банк Синара | — |

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
- SVG: a `viewBox`, no external references (`<image href="http…">`, web fonts),
  text converted to outlines. PNG fallback: **≥ 256×256**, transparent outside
  the mark.
- Optical sizing: the artwork should fill its square the way an app icon does.
  Large transparent margins make one bank look shrunken next to the others.

## Attribution

Every file added here needs a row in the repo's `ACKNOWLEDGEMENTS.md` — source
URL and rights holder. These are third-party trademarks; the repo is public.
