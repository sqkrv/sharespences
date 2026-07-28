package vision

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
)

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodedSize(t *testing.T, jpg []byte) (int, int) {
	t.Helper()
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpg))
	if err != nil {
		t.Fatalf("Prepare output is not a JPEG: %v", err)
	}
	return cfg.Width, cfg.Height
}

func TestPrepareDownscalesToLongEdge(t *testing.T) {
	out, err := Prepare(encodePNG(t, 2000, 1000), DefaultLongEdge)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := decodedSize(t, out); w != 1664 || h != 832 {
		t.Fatalf("got %dx%d, want 1664x832", w, h)
	}
	// Portrait phone screenshot shape.
	out, err = Prepare(encodePNG(t, 1290, 2796), DefaultLongEdge)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := decodedSize(t, out); h != 1664 || w != 1290*1664/2796 {
		t.Fatalf("got %dx%d, want %dx1664", w, h, 1290*1664/2796)
	}
}

func TestPrepareNeverUpscales(t *testing.T) {
	out, err := Prepare(encodePNG(t, 100, 50), DefaultLongEdge)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := decodedSize(t, out); w != 100 || h != 50 {
		t.Fatalf("got %dx%d, want 100x50 (no upscale)", w, h)
	}
}

func TestPrepareDecodesWebP(t *testing.T) {
	raw, err := os.ReadFile("testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Prepare(raw, DefaultLongEdge)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := decodedSize(t, out); w != 64 || h != 96 {
		t.Fatalf("got %dx%d, want 64x96", w, h)
	}
}

// A decompression bomb must be rejected from the header alone: this PNG
// declares 30000×30000 (≈3.6 GB decoded) in a few dozen bytes and would
// pass the 10 MiB upload cap trivially.
func TestPrepareRejectsDecompressionBomb(t *testing.T) {
	var b bytes.Buffer
	b.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], 30000) // width
	binary.BigEndian.PutUint32(ihdr[4:], 30000) // height
	ihdr[8], ihdr[9] = 8, 6                     // bit depth, RGBA
	var chunk bytes.Buffer
	chunk.WriteString("IHDR")
	chunk.Write(ihdr)
	_ = binary.Write(&b, binary.BigEndian, uint32(13))
	b.Write(chunk.Bytes())
	_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(chunk.Bytes()))

	_, err := Prepare(b.Bytes(), DefaultLongEdge)
	if !errors.Is(err, ErrBadImage) {
		t.Fatalf("want ErrBadImage, got %v", err)
	}
}

func TestPrepareRejectsUndecodable(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte("%PDF-1.4 not an image"),
		{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c'}, // HEIC magic
		nil,
	} {
		if _, err := Prepare(raw, DefaultLongEdge); !errors.Is(err, ErrBadImage) {
			t.Fatalf("want ErrBadImage for %q, got %v", raw, err)
		}
	}
}
