# Acknowledgements

Third-party material bundled with Sharespences, and where it came from.

## Логотипы банков

Bank names, logos and other brand marks are the **trademarks of their
respective owners**. They are used here solely to identify the bank a card or
cashback offer belongs to — nominative use. Sharespences is not affiliated
with, endorsed by, or sponsored by any of the banks listed below, and the
project claims no rights in these marks.

Files live in [`web/src/assets/banks/`](web/src/assets/banks/) (conventions in
that folder's README) and are bundled into the app.

| Bank | File | Source | Rights holder |
|---|---|---|---|
| Альфа-Банк | `alfabank.svg` | [rblp] `dark/svg/icon/alfabank.svg` | АО «Альфа-Банк» |
| ВТБ | `vtb.svg` | [rblp] `dark/svg/icon/vtb.svg` | Банк ВТБ (ПАО) |
| Озон Банк | `ozon.svg` | [rblp] `dark/svg/icon/ozon.svg` | ООО «Озон Банк» |
| Яндекс Пэй | `yandex-pay.svg` | [Trace Logos](https://trace-logos.ru/en/logos/pay/yandexpay/) | ООО «Яндекс» |
| Газпромбанк | `gazprombank.svg` | [rblp] `dark/svg/icon/gazprombank.svg` | «Газпромбанк» (АО) |
| МКБ | `mkb.svg` | [rblp] `dark/svg/icon/mkb.svg` | ПАО «Московский кредитный банк» |
| Сбербанк | `sber.svg` | [rblp] `dark/svg/icon/sberbank.svg` | ПАО Сбербанк |
| Т-Банк | `tbank.svg` | [rblp] `dark/svg/icon/tbank.svg` | АО «Т-Банк» |

[rblp]: https://github.com/melpnz/rblp

**[rblp]** — «286 Russian Banks Logos», a community pack of Russian bank marks
in SVG and PNG (dark/light, icon and horizontal variants). It ships **no
license file** and states that the logos remain the property of their owners,
so the pack grants no rights of its own: the basis for using these marks here
is nominative use of the banks' trademarks, as set out above, and the pack is
credited for the vector artwork. The seven bundled files are byte-identical to their upstream counterparts (verified 2026-07-27). **Trace Logos** likewise states that the logos
are trademarks of their respective owners.

A bank without a file simply falls back to a two-letter avatar in the app; new
files get a row here with the page they came from.

If you are a rights holder and want a mark removed or replaced with the
approved artwork, open an issue — it will be done.

## Merchant logos

None bundled yet. When merchant marks are added (partner offers, points of
sale), they are subject to the same terms as the bank marks above and get
their own rows here.

## Fonts

- **Golos Text** — © 2019 The Golos Text Project Authors, [SIL Open Font
  License 1.1](https://github.com/googlefonts/golos-text); vendored through
  [`@fontsource/golos-text`](https://www.npmjs.com/package/@fontsource/golos-text)
  and self-hosted so the PWA works offline.

## Data

- **MCC dictionary** — assembled from public MCC references (mcc-codes.ru and
  the banks' own published MCC appendices); per-bank category↔MCC membership is
  derived from each bank's loyalty documents. See `internal/seed/data/`.
