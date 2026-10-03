package docgen

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func decodePNG(t *testing.T, b []byte) image.Image {
	t.Helper()
	if !bytes.HasPrefix(b, pngMagic) {
		t.Fatal("output is not a PNG")
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return img
}

func TestProcessLogoBlank(t *testing.T) {
	for _, in := range [][]byte{nil, {}} {
		out, err := ProcessLogo(in)
		if err != nil {
			t.Fatal(err)
		}
		img := decodePNG(t, out)
		if img.Bounds().Dx() != 1 || img.Bounds().Dy() != 1 {
			t.Fatalf("blank is %v", img.Bounds())
		}
		if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
			t.Fatal("blank pixel is not transparent")
		}
	}
	// BlankPNG hands out copies.
	b := BlankPNG()
	b[0] = 0
	if BlankPNG()[0] != 0x89 {
		t.Fatal("BlankPNG shares its backing array")
	}
}

func TestProcessLogoTrimsTransparentBorder(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 600, 400))
	// Nearly transparent noise in the border must still be trimmed.
	img.SetNRGBA(3, 3, color.NRGBA{R: 255, A: 10})
	for y := 100; y < 150; y++ {
		for x := 50; x < 550; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 0x41, B: 0x98, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	out, err := ProcessLogo(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got := decodePNG(t, out)
	if got.Bounds().Dx() != 500 || got.Bounds().Dy() != 50 {
		t.Fatalf("trimmed to %v, want 500x50", got.Bounds())
	}
}

func TestProcessLogoTrimsWhiteBorderOfJPEG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 300, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 300; x++ {
			c := color.RGBA{R: 255, G: 255, B: 255, A: 255}
			if x >= 40 && x < 260 && y >= 60 && y < 140 {
				c = color.RGBA{R: 20, G: 30, B: 120, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	out, err := ProcessLogo(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	b := decodePNG(t, out).Bounds()
	// JPEG ringing may leave a pixel or two of the edge; allow a small margin.
	if b.Dx() < 220 || b.Dx() > 224 || b.Dy() < 80 || b.Dy() > 84 {
		t.Fatalf("trimmed to %dx%d, want about 220x80", b.Dx(), b.Dy())
	}
}

func TestProcessLogoDownscales(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4000, 1000))
	for y := 0; y < 1000; y++ {
		for x := 0; x < 4000; x++ {
			a := uint8(255)
			if (x/10+y/10)%2 == 0 {
				a = 128 // semi-transparent checker keeps the image "with alpha"
			}
			img.SetNRGBA(x, y, color.NRGBA{R: 200, G: 10, B: 10, A: a})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	out, err := ProcessLogo(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got := decodePNG(t, out)
	if got.Bounds().Dx() != MaxLogoWidth || got.Bounds().Dy() != 400 {
		t.Fatalf("downscaled to %v, want %dx400", got.Bounds(), MaxLogoWidth)
	}
	// Colour is preserved by the alpha-weighted average.
	for _, pt := range []image.Point{{800, 200}, {801, 201}, {3, 397}} {
		c := color.NRGBAModel.Convert(got.At(pt.X, pt.Y)).(color.NRGBA)
		if c.R < 195 || c.G > 15 || c.B > 15 || c.A < 128 {
			t.Fatalf("averaged colour drifted at %v: %+v", pt, c)
		}
	}
}

func TestProcessLogoRejects(t *testing.T) {
	var gif = []byte("GIF89a\x01\x00\x01\x00")
	if _, err := ProcessLogo(gif); !errors.Is(err, ErrInvalidLogo) {
		t.Errorf("GIF: %v", err)
	}
	if _, err := ProcessLogo(append(append([]byte{}, pngMagic...), "garbage"...)); !errors.Is(err, ErrInvalidLogo) {
		t.Errorf("truncated PNG: %v", err)
	}

	// Fully transparent image: nothing left after trimming.
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 20, 20))); err != nil {
		t.Fatal(err)
	}
	if _, err := ProcessLogo(buf.Bytes()); !errors.Is(err, ErrInvalidLogo) {
		t.Errorf("empty image: %v", err)
	}

	// Decompression bomb: a tiny file whose header claims 20000x20000.
	bomb := pngWithSize(t, 20000, 20000)
	if _, err := ProcessLogo(bomb); !errors.Is(err, ErrInvalidLogo) {
		t.Errorf("bomb: %v", err)
	}
}

// pngWithSize encodes a 1x1 PNG and rewrites its IHDR dimensions.
func pngWithSize(t *testing.T, w, h uint32) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	// signature(8) + length(4) + "IHDR"(4) + data(13) + crc(4)
	binary.BigEndian.PutUint32(b[16:20], w)
	binary.BigEndian.PutUint32(b[20:24], h)
	binary.BigEndian.PutUint32(b[29:33], crc32.ChecksumIEEE(b[12:29]))
	return b
}

// masterLogoPath returns a real tenant logo kept outside the repository
// (file/ is never committed), or "" when none is available. The path comes
// from DOCGEN_LOGO_FIXTURE, else the first file/Logo_Master_*.png, so no
// client file name is written down here.
func masterLogoPath() string {
	if p := os.Getenv("DOCGEN_LOGO_FIXTURE"); p != "" {
		return p
	}
	matches, _ := filepath.Glob(filepath.Join("..", "..", "..", "file", "Logo_Master_*.png"))
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func TestProcessLogoMasterFile(t *testing.T) {
	path := masterLogoPath()
	if path == "" {
		t.Skip("master logo not available (set DOCGEN_LOGO_FIXTURE)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skip("master logo not available: ", err)
	}
	in, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	out, err := ProcessLogo(raw)
	if err != nil {
		t.Fatal(err)
	}
	b := decodePNG(t, out).Bounds()
	inRatio := float64(in.Width) / float64(in.Height)
	outRatio := float64(b.Dx()) / float64(b.Dy())
	t.Logf("master logo %dx%d (%.2f:1) -> %dx%d (%.2f:1), %d bytes", in.Width, in.Height, inRatio, b.Dx(), b.Dy(), outRatio, len(out))
	if b.Dx() > MaxLogoWidth || b.Dy() > MaxLogoHeight {
		t.Fatalf("%dx%d exceeds %dx%d", b.Dx(), b.Dy(), MaxLogoWidth, MaxLogoHeight)
	}
	if outRatio <= inRatio {
		t.Fatalf("trimming did not widen the ratio (%.2f -> %.2f)", inRatio, outRatio)
	}

	tpl := mustLoad(t, TemplateSlip)
	embedded, cx, cy, err := tpl.placeLogo(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, out) {
		t.Fatal("the stored (processed) logo must be embedded unchanged")
	}
	if cx > tpl.logo.cx || cy > tpl.logo.cy {
		t.Fatalf("logo %dx%d EMU overflows the slot", cx, cy)
	}
	if got := float64(cx) / float64(cy); got < outRatio*0.99 || got > outRatio*1.01 {
		t.Fatalf("slot ratio %.3f distorts the logo ratio %.3f", got, outRatio)
	}
}

// TestPlaceLogoUsesStoredLogoAsIs: a transparent upload whose trimmed
// content is opaque (a white card on a transparent canvas) is stored as an
// opaque PNG. Trimming it again would cut the card down to its mark, so the
// render must embed the stored bytes unchanged.
func TestPlaceLogoUsesStoredLogoAsIs(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 600, 300))
	for y := 50; y < 250; y++ {
		for x := 100; x < 500; x++ {
			c := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			if x >= 200 && x < 400 && y >= 120 && y < 180 {
				c = color.NRGBA{R: 0x41, B: 0x98, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	stored, err := ProcessLogo(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if b := decodePNG(t, stored).Bounds(); b.Dx() != 400 || b.Dy() != 200 {
		t.Fatalf("stored logo %v, want 400x200", b)
	}
	tpl := mustLoad(t, TemplateSlip)
	embedded, cx, cy, err := tpl.placeLogo(stored)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embedded, stored) {
		t.Fatal("placeLogo re-processed the stored logo")
	}
	if wantCX, wantCY := fitExtent(tpl.logo.cx, tpl.logo.cy, 400, 200); cx != wantCX || cy != wantCY {
		t.Fatalf("extent %dx%d, want %dx%d", cx, cy, wantCX, wantCY)
	}

	// Raw uploads are not accepted at render time: JPEG, or a PNG larger
	// than ProcessLogo ever produces.
	var jbuf bytes.Buffer
	if err := jpeg.Encode(&jbuf, img, nil); err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{"jpeg": jbuf.Bytes(), "oversized png": pngWithSize(t, MaxLogoWidth+1, 10)} {
		if _, _, _, err := tpl.placeLogo(raw); !errors.Is(err, ErrInvalidLogo) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestProcessLogoBoundsHeight(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 500, 3000))
	for y := 0; y < 3000; y++ {
		for x := 0; x < 500; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 10, G: 120, B: 10, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	out, err := ProcessLogo(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if b := decodePNG(t, out).Bounds(); b.Dx() != 100 || b.Dy() != MaxLogoHeight {
		t.Fatalf("tall logo downscaled to %v, want 100x%d", b, MaxLogoHeight)
	}
}

func TestProcessLogoSizeCaps(t *testing.T) {
	for _, size := range [][2]uint32{{maxLogoSide + 1, 10}, {10, maxLogoSide + 1}, {4000, 4000}} {
		if _, err := ProcessLogo(pngWithSize(t, size[0], size[1])); !errors.Is(err, ErrInvalidLogo) {
			t.Errorf("%dx%d accepted: %v", size[0], size[1], err)
		}
	}
}

func TestFitPixels(t *testing.T) {
	cases := []struct{ w, h, wantW, wantH int }{
		{4000, 1000, 1600, 400},
		{1000, 4000, 150, 600},
		{1600, 600, 1600, 600},
		{100000, 1, 1600, 1},
		{1, 100000, 1, 600},
	}
	for _, c := range cases {
		if w, h := fitPixels(c.w, c.h, MaxLogoWidth, MaxLogoHeight); w != c.wantW || h != c.wantH {
			t.Errorf("fitPixels(%d, %d) = %dx%d, want %dx%d", c.w, c.h, w, h, c.wantW, c.wantH)
		}
	}
}
