package favicon

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func pngBytes(t *testing.T, size int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// icoWith wraps raw image payloads into an .ico container.
func icoWith(entries ...[]byte) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, [3]uint16{0, 1, uint16(len(entries))})
	offset := 6 + 16*len(entries)
	for _, e := range entries {
		binary.Write(&buf, binary.LittleEndian, struct {
			W, H, C, R byte
			Planes     uint16
			BPP        uint16
			Size       uint32
			Offset     uint32
		}{32, 32, 0, 0, 1, 32, uint32(len(e)), uint32(offset)})
		offset += len(e)
	}
	for _, e := range entries {
		buf.Write(e)
	}
	return buf.Bytes()
}

// dib32 is a 2×2 32-bit BGRA bitmap as stored in .ico files (bottom-up, height doubled).
func dib32() []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, struct {
		Size                  uint32
		W, H                  int32
		Planes, BPP           uint16
		Comp, ImgSize         uint32
		XPPM, YPPM            int32
		ColorsUsed, Important uint32
	}{40, 2, 4, 1, 32, 0, 0, 0, 0, 0, 0})
	// bottom row: blue, transparent; top row: red, green
	buf.Write([]byte{255, 0, 0, 255, 0, 0, 0, 0})
	buf.Write([]byte{0, 0, 255, 255, 0, 255, 0, 255})
	buf.Write(make([]byte, 8)) // AND mask, 2 rows × 4 bytes
	return buf.Bytes()
}

func TestDecodeICO(t *testing.T) {
	img, err := Decode(icoWith(dib32()))
	if err != nil {
		t.Fatal(err)
	}
	at := func(x, y int) color.NRGBA { return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA) }
	if at(0, 0) != (color.NRGBA{255, 0, 0, 255}) || at(1, 0) != (color.NRGBA{0, 255, 0, 255}) || at(0, 1) != (color.NRGBA{0, 0, 255, 255}) || at(1, 1).A != 0 {
		t.Fatalf("pixels: %v %v %v %v", at(0, 0), at(1, 0), at(0, 1), at(1, 1))
	}
	// PNG-in-ICO, the modern form.
	img, err = Decode(icoWith(pngBytes(t, 48, color.NRGBA{1, 2, 3, 255})))
	if err != nil || img.Bounds().Dx() != 48 {
		t.Fatalf("png entry: %v", err)
	}
	if _, err := Decode([]byte("not an image")); err == nil {
		t.Fatal("garbage must fail")
	}
}

func TestFetchPrefersTheBiggestIcon(t *testing.T) {
	big := pngBytes(t, 180, color.NRGBA{255, 0, 0, 255})
	small := icoWith(pngBytes(t, 32, color.NRGBA{0, 0, 255, 255}))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><head>
			<link rel="icon" href="/static/fav.ico" sizes="32x32">
			<link rel="icon" type="image/svg+xml" href="/logo.svg">
			<link rel="apple-touch-icon" sizes="180x180" href="static/touch.png">
		</head><body><link rel="icon" href="/ignored.png"></body></html>`)
	})
	mux.HandleFunc("/static/touch.png", func(w http.ResponseWriter, r *http.Request) { w.Write(big) })
	mux.HandleFunc("/static/fav.ico", func(w http.ResponseWriter, r *http.Request) { w.Write(small) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	img, err := Fetch(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 180 {
		t.Fatalf("got %dpx, want the 180px touch icon", img.Bounds().Dx())
	}
}

func TestFetchFallsBackToFaviconICO(t *testing.T) {
	ico := icoWith(pngBytes(t, 32, color.NRGBA{0, 0, 255, 255}))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/favicon.ico":
			w.Write(ico)
		case "/app":
			w.WriteHeader(http.StatusForbidden) // pages behind a login still have /favicon.ico
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	img, err := Fetch(context.Background(), srv.URL+"/app")
	if err != nil || img.Bounds().Dx() != 32 {
		t.Fatalf("fallback: %v", err)
	}
	if _, err := Fetch(context.Background(), "steam://rungameid/570"); err != ErrNoIcon {
		t.Fatalf("custom schemes have no site icon: %v", err)
	}
}
