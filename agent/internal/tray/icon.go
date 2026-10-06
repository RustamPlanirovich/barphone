package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"

	"golang.org/x/image/draw"
)

// The app glyph (a 3x2 deck of keys) is drawn in code so no binary assets are needed.

func roundedRect(img *image.NRGBA, x0, y0, x1, y1, r float64, c color.NRGBA) {
	for y := int(y0); y < int(y1+1); y++ {
		for x := int(x0); x < int(x1+1); x++ {
			px, py := float64(x)+.5, float64(y)+.5
			if px < x0 || px > x1 || py < y0 || py > y1 {
				continue
			}
			cx := min(max(px, x0+r), x1-r)
			cy := min(max(py, y0+r), y1-r)
			if (px-cx)*(px-cx)+(py-cy)*(py-cy) <= r*r {
				img.SetNRGBA(x, y, c)
			}
		}
	}
}

// Glyph renders the icon at size x size pixels (supersampled for smooth edges).
func Glyph(size int) image.Image {
	const ss = 4
	s := float64(size * ss)
	big := image.NewNRGBA(image.Rect(0, 0, size*ss, size*ss))
	u := s / 64
	roundedRect(big, 2*u, 2*u, 62*u, 62*u, 14*u, color.NRGBA{0x16, 0x18, 0x1d, 0xff})
	hot := color.NRGBA{0xff, 0x7a, 0x45, 0xff}
	soft := color.NRGBA{0xff, 0xb3, 0x8f, 0xff}
	keys := []struct {
		x, y float64
		c    color.NRGBA
	}{{12, 14, hot}, {26.5, 14, soft}, {41, 14, hot}, {12, 29, soft}, {26.5, 29, hot}, {41, 29, soft}}
	for _, k := range keys {
		roundedRect(big, k.x*u, k.y*u, (k.x+11)*u, (k.y+11)*u, 3*u, k.c)
	}
	roundedRect(big, 22*u, 47*u, 42*u, 51*u, 2*u, color.NRGBA{0x5b, 0x61, 0x6e, 0xff})

	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.BiLinear.Scale(out, out.Bounds(), big, big.Bounds(), draw.Src, nil)
	return out
}

func GlyphPNG(size int) []byte {
	var buf bytes.Buffer
	png.Encode(&buf, Glyph(size))
	return buf.Bytes()
}

// GlyphICO wraps PNG-encoded frames into a .ico container (supported since Vista).
func GlyphICO(sizes ...int) []byte {
	frames := make([][]byte, len(sizes))
	for i, s := range sizes {
		frames[i] = GlyphPNG(s)
	}
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim := byte(s)
		if s >= 256 {
			dim = 0
		}
		binary.Write(&buf, binary.LittleEndian, struct {
			W, H, Colors, Reserved byte
			Planes, BPP            uint16
			Size, Offset           uint32
		}{dim, dim, 0, 0, 1, 32, uint32(len(frames[i])), uint32(offset)})
		offset += len(frames[i])
	}
	for _, f := range frames {
		buf.Write(f)
	}
	return buf.Bytes()
}
