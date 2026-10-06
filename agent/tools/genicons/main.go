// Command genicons renders the barphone glyph into app icons, so the phone app and the
// PC agent share one drawing:
//
//	go run ./tools/genicons ../app                    # Android launcher icons
//	go run ./tools/genicons -iconset <dir>.iconset    # macOS icon set for iconutil
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"barphone/agent/internal/tray"
)

func main() {
	switch {
	case len(os.Args) == 3 && os.Args[1] == "-iconset":
		iconset(os.Args[2])
	case len(os.Args) == 2:
		android(os.Args[1])
	default:
		fmt.Fprintln(os.Stderr, "usage: genicons <path to Flutter app> | genicons -iconset <dir>.iconset")
		os.Exit(2)
	}
}

func android(app string) {
	res := filepath.Join(app, "android", "app", "src", "main", "res")
	sizes := map[string]int{"mdpi": 48, "hdpi": 72, "xhdpi": 96, "xxhdpi": 144, "xxxhdpi": 192}
	for density, size := range sizes {
		write(filepath.Join(res, "mipmap-"+density, "ic_launcher.png"), tray.GlyphPNG(size))
	}
}

// iconset writes the file names iconutil expects. The glyph is inset to Apple's icon grid
// (824 of 1024 px), otherwise it looks oversized next to other apps in the Dock and Finder.
func iconset(dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err)
	}
	for _, pt := range []int{16, 32, 128, 256, 512} {
		for scale, suffix := range map[int]string{1: "", 2: "@2x"} {
			size := pt * scale
			glyph := tray.Glyph(size * 824 / 1024)
			canvas := image.NewNRGBA(image.Rect(0, 0, size, size))
			off := (size - glyph.Bounds().Dx()) / 2
			draw.Draw(canvas, glyph.Bounds().Add(image.Pt(off, off)), glyph, image.Point{}, draw.Src)
			var buf bytes.Buffer
			png.Encode(&buf, canvas)
			write(filepath.Join(dir, fmt.Sprintf("icon_%dx%d%s.png", pt, pt, suffix)), buf.Bytes())
		}
	}
}

func write(path string, data []byte) {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fail(err)
	}
	fmt.Println("wrote", path)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
