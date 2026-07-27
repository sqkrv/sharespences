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
| Альфа-Банк | `alfabank.svg` | _to fill_ | АО «Альфа-Банк» |
| ВТБ | `vtb.svg` | _to fill_ | Банк ВТБ (ПАО) |
| Озон Банк | `ozon.svg` | _to fill_ | ООО «Озон Банк» |
| Яндекс Пэй | `yandex-pay.svg` | _to fill_ | ООО «Яндекс» |
| Газпромбанк | `gazprombank.svg` | _to fill_ | «Газпромбанк» (АО) |
| МКБ | `mkb.svg` | _to fill_ | ПАО «Московский кредитный банк» |
| Сбербанк | `sber.svg` | _to fill_ | ПАО Сбербанк |
| Т-Банк | `tbank.svg` | _to fill_ | АО «Т-Банк» |

Fill the Source column with the page the file was downloaded from (a brand /
press kit page where one exists) as each logo lands; rows whose file is absent
simply fall back to a two-letter avatar in the app.

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
