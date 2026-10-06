// Command genicons renders the barphone glyph into the Android launcher icons, so the
// phone app and the PC agent share one drawing:  go run ./tools/genicons ../app
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"barphone/agent/internal/tray"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: genicons <path to Flutter app>")
		os.Exit(2)
	}
	res := filepath.Join(os.Args[1], "android", "app", "src", "main", "res")
	sizes := map[string]int{"mdpi": 48, "hdpi": 72, "xhdpi": 96, "xxhdpi": 144, "xxxhdpi": 192}
	for density, size := range sizes {
		p := filepath.Join(res, "mipmap-"+density, "ic_launcher.png")
		if err := os.WriteFile(p, tray.GlyphPNG(size), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("wrote", p)
	}
}
