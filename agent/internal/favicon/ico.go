package favicon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
)

// decodeICO picks the largest image in a .ico file. Entries are either PNG (modern
// favicons) or a headerless BMP: 32-bit BGRA, or 24/8/4/1-bit with an AND transparency mask.
func decodeICO(data []byte) (image.Image, error) {
	if len(data) < 6 {
		return nil, errors.New("ico: short header")
	}
	count := int(binary.LittleEndian.Uint16(data[4:]))
	type entry struct {
		w, bpp       int
		size, offset uint32
	}
	var best *entry
	for i := 0; i < count; i++ {
		off := 6 + 16*i
		if off+16 > len(data) {
			break
		}
		e := entry{
			w:      int(data[off]),
			bpp:    int(binary.LittleEndian.Uint16(data[off+6:])),
			size:   binary.LittleEndian.Uint32(data[off+8:]),
			offset: binary.LittleEndian.Uint32(data[off+12:]),
		}
		if e.w == 0 {
			e.w = 256
		}
		if uint64(e.offset)+uint64(e.size) > uint64(len(data)) {
			continue
		}
		if best == nil || e.w > best.w || (e.w == best.w && e.bpp > best.bpp) {
			best = &e
		}
	}
	if best == nil {
		return nil, errors.New("ico: no images")
	}
	img := data[best.offset : best.offset+best.size]
	if bytes.HasPrefix(img, []byte("\x89PNG")) {
		return png.Decode(bytes.NewReader(img))
	}
	return decodeDIB(img)
}

func decodeDIB(b []byte) (image.Image, error) {
	if len(b) < 40 {
		return nil, errors.New("ico: short bitmap")
	}
	hdr := int(binary.LittleEndian.Uint32(b))
	w := int(int32(binary.LittleEndian.Uint32(b[4:])))
	h := int(int32(binary.LittleEndian.Uint32(b[8:]))) / 2 // XOR image + AND mask
	bpp := int(binary.LittleEndian.Uint16(b[14:]))
	colors := int(binary.LittleEndian.Uint32(b[32:]))
	if w <= 0 || h <= 0 || w > 512 || h > 512 || hdr < 40 {
		return nil, errors.New("ico: bad bitmap size")
	}
	var palette []color.NRGBA
	if bpp <= 8 {
		if colors == 0 {
			colors = 1 << bpp
		}
		for i := 0; i < colors; i++ {
			p := hdr + 4*i
			if p+4 > len(b) {
				return nil, errors.New("ico: short palette")
			}
			palette = append(palette, color.NRGBA{R: b[p+2], G: b[p+1], B: b[p], A: 255})
		}
	}
	stride := ((w*bpp + 31) / 32) * 4
	pixels := hdr + 4*len(palette)
	maskStride := ((w + 31) / 32) * 4
	mask := pixels + stride*h
	if mask > len(b) {
		return nil, errors.New("ico: short pixel data")
	}
	hasMask := mask+maskStride*h <= len(b)

	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	anyAlpha := false
	for y := 0; y < h; y++ {
		row := b[pixels+(h-1-y)*stride:] // rows are stored bottom-up
		for x := 0; x < w; x++ {
			var c color.NRGBA
			switch bpp {
			case 32:
				c = color.NRGBA{R: row[4*x+2], G: row[4*x+1], B: row[4*x], A: row[4*x+3]}
				if c.A != 0 {
					anyAlpha = true
				}
			case 24:
				c = color.NRGBA{R: row[3*x+2], G: row[3*x+1], B: row[3*x], A: 255}
			case 8, 4, 1:
				bit := x * bpp
				idx := int(row[bit/8]>>(8-bpp-bit%8)) & (1<<bpp - 1)
				if idx < len(palette) {
					c = palette[idx]
				}
			default:
				return nil, errors.New("ico: unsupported bit depth")
			}
			out.SetNRGBA(x, y, c)
		}
	}
	// 32-bit icons carry their own alpha; the others use the AND mask (1 = transparent).
	if hasMask && (bpp != 32 || !anyAlpha) {
		for y := 0; y < h; y++ {
			mrow := b[mask+(h-1-y)*maskStride:]
			for x := 0; x < w; x++ {
				transparent := mrow[x/8]&(0x80>>(x%8)) != 0
				c := out.NRGBAAt(x, y)
				if transparent {
					c.A = 0
				} else {
					c.A = 255
				}
				out.SetNRGBA(x, y, c)
			}
		}
	}
	return out, nil
}
