package docgen

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"sync"
)

// Tenant logo processing. Logos are uploaded once (Admin > Settings > Profil
// Perusahaan), processed here and stored as PNG; renders embed them in the
// payslip logo slot, aspect-fitted into the template's original box.

const (
	// MaxLogoWidth and MaxLogoHeight bound a processed logo: a trimmed logo
	// larger than either is downscaled proportionally. The payslip slot is
	// 5.03 x 1.08 cm (4.63:1), so 1600 x 600 px is well above print
	// resolution for any aspect ratio.
	MaxLogoWidth  = 1600
	MaxLogoHeight = 600
	// maxLogoPixels and maxLogoSide bound the decoded size (decompression
	// bombs): a 2 MB upload can otherwise expand to gigabytes. Print master
	// logos are commonly ~6000 x 2000 px; 12 MP keeps the worst case (a
	// 16-bit decode plus its NRGBA copy, 12 bytes per pixel) under 150 MB,
	// and logoSem allows only one such decode at a time.
	maxLogoPixels = 12_000_000
	maxLogoSide   = 8_000
)

// logoSem serialises the decode/trim/encode work of ProcessLogo so parallel
// uploads cannot multiply its peak memory.
var logoSem = make(chan struct{}, 1)

// ErrInvalidLogo is returned for anything that is not a usable PNG or JPEG.
var ErrInvalidLogo = errors.New("logo harus berupa gambar PNG atau JPEG yang valid")

var (
	pngMagic  = []byte("\x89PNG\r\n\x1a\n")
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
)

// ProcessLogo decodes a PNG or JPEG logo, trims its uniform border (pixels
// with alpha < 16 when the image has transparency, otherwise near-white
// pixels with r, g, b >= 245), downscales it proportionally to at most
// MaxLogoWidth x MaxLogoHeight pixels (area average) and re-encodes it as
// PNG. nil or empty input yields a 1x1 transparent PNG, which leaves the
// slot empty. It runs once, at upload; renders embed its output as is
// (WithLogo), because trimming is not idempotent (a transparent logo whose
// trimmed content is opaque is re-encoded without alpha).
func ProcessLogo(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return BlankPNG(), nil
	}
	var (
		decode       func(*bytes.Reader) (image.Image, error)
		decodeConfig func(*bytes.Reader) (image.Config, error)
	)
	switch {
	case bytes.HasPrefix(data, pngMagic):
		decode = func(r *bytes.Reader) (image.Image, error) { return png.Decode(r) }
		decodeConfig = func(r *bytes.Reader) (image.Config, error) { return png.DecodeConfig(r) }
	case bytes.HasPrefix(data, jpegMagic):
		decode = func(r *bytes.Reader) (image.Image, error) { return jpeg.Decode(r) }
		decodeConfig = func(r *bytes.Reader) (image.Config, error) { return jpeg.DecodeConfig(r) }
	default:
		return nil, ErrInvalidLogo
	}

	cfg, err := decodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLogo, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxLogoSide || cfg.Height > maxLogoSide ||
		int64(cfg.Width)*int64(cfg.Height) > maxLogoPixels {
		return nil, fmt.Errorf("%w: ukuran %dx%d px terlalu besar", ErrInvalidLogo, cfg.Width, cfg.Height)
	}
	logoSem <- struct{}{}
	defer func() { <-logoSem }()
	src, err := decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLogo, err)
	}

	img := toNRGBA(src)
	hasAlpha := true
	if o, ok := src.(interface{ Opaque() bool }); ok && o.Opaque() {
		hasAlpha = false
	}
	bounds, ok := contentBounds(img, hasAlpha)
	if !ok {
		return nil, fmt.Errorf("%w: gambar kosong", ErrInvalidLogo)
	}
	trimmed := img.SubImage(bounds).(*image.NRGBA)

	var out image.Image = trimmed
	if w, h := bounds.Dx(), bounds.Dy(); w > MaxLogoWidth || h > MaxLogoHeight {
		nw, nh := fitPixels(w, h, MaxLogoWidth, MaxLogoHeight)
		out = downscale(trimmed, nw, nh)
	}

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, out); err != nil {
		return nil, fmt.Errorf("encode logo: %w", err)
	}
	return buf.Bytes(), nil
}

var blankPNG = sync.OnceValue(func() []byte {
	var buf bytes.Buffer
	// A 1x1 fully transparent NRGBA image cannot fail to encode.
	_ = png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	return buf.Bytes()
})

// BlankPNG returns a 1x1 transparent PNG.
func BlankPNG() []byte {
	b := blankPNG()
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// contentBounds returns the bounding box of the non-background pixels.
func contentBounds(img *image.NRGBA, hasAlpha bool) (image.Rectangle, bool) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	isBG := func(p []uint8) bool {
		if hasAlpha {
			return p[3] < 16
		}
		return p[0] >= 245 && p[1] >= 245 && p[2] >= 245
	}
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+w*4]
		for x := 0; x < w; x++ {
			if isBG(row[x*4 : x*4+4]) {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < 0 {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
}

// downscale resizes src to w x h by averaging the source pixels that fall in
// each destination pixel's box (colour weighted by alpha so transparent
// pixels do not darken edges).
func downscale(src *image.NRGBA, w, h int) *image.NRGBA {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for dy := 0; dy < h; dy++ {
		y0 := dy * sh / h
		y1 := (dy + 1) * sh / h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for dx := 0; dx < w; dx++ {
			x0 := dx * sw / w
			x1 := (dx + 1) * sw / w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, a, n uint64
			for y := y0; y < y1; y++ {
				// Pix of a sub-image starts at its Rect.Min, so rows are
				// addressed relative to the sub-image.
				row := src.Pix[y*src.Stride:]
				for x := x0; x < x1; x++ {
					p := row[x*4 : x*4+4]
					pa := uint64(p[3])
					r += uint64(p[0]) * pa
					g += uint64(p[1]) * pa
					b += uint64(p[2]) * pa
					a += pa
					n++
				}
			}
			o := dy*dst.Stride + dx*4
			if a > 0 {
				dst.Pix[o] = uint8(r / a)
				dst.Pix[o+1] = uint8(g / a)
				dst.Pix[o+2] = uint8(b / a)
			}
			dst.Pix[o+3] = uint8(a / n)
		}
	}
	return dst
}

// fitPixels returns the largest size with the aspect ratio of w x h that
// fits in maxW x maxH (at least 1 px per side).
func fitPixels(w, h, maxW, maxH int) (int, int) {
	if int64(w)*int64(maxH) >= int64(h)*int64(maxW) {
		nh := int((int64(h)*int64(maxW) + int64(w)/2) / int64(w))
		return maxW, max(nh, 1)
	}
	nw := int((int64(w)*int64(maxH) + int64(h)/2) / int64(h))
	return max(nw, 1), maxH
}

// fitExtent scales an image of w x h pixels to the largest size that fits in
// the slot (EMU) while keeping its aspect ratio.
func fitExtent(slotCX, slotCY, w, h int64) (int64, int64) {
	if w <= 0 || h <= 0 {
		return slotCX, slotCY
	}
	// Compare w/h with slotCX/slotCY without floating point.
	if w*slotCY >= h*slotCX {
		cy := (slotCX*h + w/2) / w
		if cy < 1 {
			cy = 1
		}
		return slotCX, cy
	}
	cx := (slotCY*w + h/2) / h
	if cx < 1 {
		cx = 1
	}
	return cx, slotCY
}
