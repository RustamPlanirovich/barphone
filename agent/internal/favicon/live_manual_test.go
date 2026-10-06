//go:build manual

package favicon

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fetches icons of real sites (network): go test -tags manual -run LiveSites -v ./internal/favicon
// BARPHONE_ICON_DIR keeps the PNGs for eyeballing.
func TestLiveSites(t *testing.T) {
	dir := os.Getenv("BARPHONE_ICON_DIR")
	for _, u := range []string{"https://claude.ai", "https://www.youtube.com", "https://github.com", "https://mail.google.com"} {
		img, err := Fetch(context.Background(), u)
		if err != nil {
			t.Errorf("%s: %v", u, err)
			continue
		}
		t.Logf("%-28s %v", u, img.Bounds().Size())
		if dir != "" {
			f, _ := os.Create(filepath.Join(dir, strings.NewReplacer("https://", "", "/", "_").Replace(u)+".png"))
			png.Encode(f, img)
			f.Close()
		}
	}
}
