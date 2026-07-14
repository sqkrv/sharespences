#!/usr/bin/env python3
"""Regenerate web/public/ icons from the logo. Manual one-off (needs Pillow):

    python3 web/scripts/gen-icons.py ../path/to/ss-draft-logo.png

Source lives in the private meta-repo (`docs/raw/ss-draft-logo.png`), which is
why it isn't committed here — only the derived PNGs are.

Why PNG and not SVG: `apple-touch-icon` is PNG-only (Safari ignores SVG), and
Chromium's install/splash pipeline rasterizes manifest icons — SVG there is
not dependable. So raster is the format; this script is the source of truth.

Two compositions, on purpose:
  * «any» + apple-touch — the logo as drawn: the S's bleed off the tile edge,
    which is the design. apple-touch is opaque and full-bleed (iOS applies its
    own squircle; a pre-rounded icon with transparent corners would get
    double-rounded / black corners).
  * maskable — Android masks to a circle/squircle and only guarantees the
    central 80% («safe zone»). The logo's S's touch the right/bottom edges, so
    a full-bleed maskable gets them chopped by a round mask. Here the artwork
    is scaled so the *glyph* bounding box fits the safe circle and is centred,
    with the tile's own background gradient extended around it.
"""

import math
import sys
from pathlib import Path

from PIL import Image, ImageDraw

OUT = Path(__file__).resolve().parent.parent / "public"
BG_TOP, BG_BOTTOM = (19, 14, 43), (10, 6, 20)  # sampled from the tile's gradient
GLYPH_MIN_BRIGHTNESS = 90  # S's are lilac/coral; the tile background is near-black
SAFE_ZONE = 0.4  # maskable: content must fit a centred circle of r = 40% of the canvas


def squared(src: Image.Image) -> Image.Image:
    side = max(src.size)
    sq = Image.new("RGBA", (side, side), (0, 0, 0, 0))
    sq.paste(src, ((side - src.width) // 2, (side - src.height) // 2))
    return sq


def glyph_bbox(img: Image.Image) -> tuple[int, int, int, int]:
    px = img.load()
    xs, ys = [], []
    for y in range(0, img.height, 2):
        for x in range(0, img.width, 2):
            r, g, b, a = px[x, y]
            if a > 128 and max(r, g, b) > GLYPH_MIN_BRIGHTNESS:
                xs.append(x)
                ys.append(y)
    return min(xs), min(ys), max(xs), max(ys)


def background(size: int) -> Image.Image:
    img = Image.new("RGB", (size, size))
    d = ImageDraw.Draw(img)
    for y in range(size):
        t = y / (size - 1)
        d.line([(0, y), (size, y)], fill=tuple(round(a + (b - a) * t) for a, b in zip(BG_TOP, BG_BOTTOM)))
    return img


def full_bleed(sq: Image.Image, size: int) -> Image.Image:
    """Opaque square: artwork edge-to-edge, tile corners filled with its background."""
    canvas = background(size)
    art = sq.resize((size, size), Image.LANCZOS)
    canvas.paste(art, (0, 0), art)
    return canvas


def maskable(sq: Image.Image, size: int) -> Image.Image:
    x0, y0, x1, y1 = glyph_bbox(sq)
    scale = (SAFE_ZONE * size) / math.hypot((x1 - x0) / 2, (y1 - y0) / 2)
    tile = round(sq.width * scale)
    art = sq.resize((tile, tile), Image.LANCZOS)
    offset = (
        round(size / 2 - (x0 + x1) / 2 * scale),  # centre the GLYPHS, not the tile:
        round(size / 2 - (y0 + y1) / 2 * scale),  # they sit right/bottom of centre
    )
    canvas = background(size)
    canvas.paste(art, offset, art)
    return canvas


def main() -> None:
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    sq = squared(Image.open(sys.argv[1]).convert("RGBA"))

    # «any»: the logo as drawn (rounded corners stay transparent).
    for size in (192, 512):
        sq.resize((size, size), Image.LANCZOS).save(OUT / f"pwa-{size}.png")
    sq.resize((64, 64), Image.LANCZOS).save(OUT / "favicon.png")
    maskable(sq, 512).save(OUT / "pwa-maskable-512.png")
    full_bleed(sq, 180).save(OUT / "apple-touch-icon.png")
    for f in sorted(OUT.glob("*.png")):
        print(f"{f.name}: {Image.open(f).size} {Image.open(f).mode}")


if __name__ == "__main__":
    main()
