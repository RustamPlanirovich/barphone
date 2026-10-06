//go:build windows && manual

package launch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"barphone/agent/internal/icons"
	"barphone/agent/internal/store"
)

// go test -tags manual -run AppsAndIcons -v ./internal/launch
// Set BARPHONE_ICON_DIR to keep the extracted PNGs for eyeballing.
func TestAppsAndIcons(t *testing.T) {
	l := New()
	start := time.Now()
	apps, err := l.Apps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d apps in %v", len(apps), time.Since(start))
	if len(apps) == 0 {
		t.Fatal("no apps")
	}

	dir := os.Getenv("BARPHONE_ICON_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	st, err := icons.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var picked []App
	for _, a := range apps {
		n := strings.ToLower(a.Name)
		if strings.Contains(n, "калькулятор") || strings.Contains(n, "calculator") || strings.Contains(n, "notepad") ||
			strings.Contains(n, "блокнот") || strings.Contains(n, "edge") || strings.Contains(n, "code") || strings.Contains(n, "проводник") {
			picked = append(picked, a)
		}
	}
	picked = append(picked, apps[:3]...)
	picked = append(picked, App{Name: "cmd (path)", Kind: store.KindPath, Target: `%windir%\system32\cmd.exe`})
	picked = append(picked, App{Name: "folder (path)", Kind: store.KindPath, Target: `C:\Windows`})
	for _, a := range picked {
		s := time.Now()
		img, err := l.Icon(a.Kind, a.Target)
		if err != nil {
			t.Errorf("%s (%s): %v", a.Name, a.Target, err)
			continue
		}
		hash, err := st.PutImage(img)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%-40s %-60s %v %s %v", a.Name, a.Target, img.Bounds().Size(), filepath.Join(dir, hash+".png"), time.Since(s))
	}
}
