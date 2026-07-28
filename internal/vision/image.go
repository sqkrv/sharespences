package vision

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	// Decoders for the formats the SPA picker offers. HEIC and PDF are on
	// the attachment upload allowlist but decodable by nothing in Go —
	// Prepare rejects them with ErrBadImage and the job skips the image
	// with a visible note (plan contract C3).
	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	// DefaultLongEdge mirrors the harness LONG_EDGE: the benchmarked
	// accuracy numbers were measured at 1664px.
	DefaultLongEdge = 1664
	// RetryLongEdge is the reduced-resolution retry after the OOM
	// signature (run 5: HTTP 500 «unexpected EOF» on the card-grid shot,
	// recovered at 1024).
	RetryLongEdge = 1024
	// maxDecodedPixels bounds the decode BEFORE it happens: the 10 MiB
	// upload cap does not bound decoded size — a 30000×30000 PNG fits
	// under it while decoding to ~3.6 GB in the process that also serves
	// every other feature.
	maxDecodedPixels = 40 << 20 // 40 MPx
	jpegQuality      = 88       // harness parity
)

// Prepare decodes an uploaded screenshot, downscales it so the long edge
// is at most longEdge, and re-encodes as JPEG for the backend. It never
// upscales. EXIF orientation is deliberately not applied: the SPA picker
// is narrowed to screenshot formats, and screenshots carry no rotation.
func Prepare(raw []byte, longEdge int) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadImage, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxDecodedPixels {
		return nil, fmt.Errorf("%w: %dx%d exceeds the %d-megapixel decode bound", ErrBadImage, cfg.Width, cfg.Height, maxDecodedPixels>>20)
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadImage, err)
	}

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if longEdge > 0 && (w > longEdge || h > longEdge) {
		if w >= h {
			h = h * longEdge / w
			w = longEdge
		} else {
			w = w * longEdge / h
			h = longEdge
		}
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
		src = dst
	}

	var out bytes.Buffer
	if err := jpeg.Encode(&out, src, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("%w: re-encode: %v", ErrBadImage, err)
	}
	return out.Bytes(), nil
}
