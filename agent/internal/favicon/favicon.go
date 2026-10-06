// Package favicon finds the best icon of a website for link buttons: apple-touch-icon,
// then the largest <link rel="icon">, then /favicon.ico.
package favicon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
	"golang.org/x/net/html"
)

const (
	maxPage  = 1 << 20 // bytes of HTML to look at
	maxImage = 2 << 20
	timeout  = 8 * time.Second
	// Some sites only send icons to browsers.
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36 barphone"
)

var ErrNoIcon = errors.New("no usable icon")

var client = &http.Client{Timeout: timeout}

type candidate struct {
	url   string
	score int // higher is better
}

// Fetch returns the site's icon for an http(s) URL.
func Fetch(ctx context.Context, rawURL string) (image.Image, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, ErrNoIcon // custom schemes (steam://, tg://) have no website
	}
	ctx, cancel := context.WithTimeout(ctx, 3*timeout)
	defer cancel()

	var cands []candidate
	if body, final, err := get(ctx, u.String(), maxPage); err == nil {
		cands = parseLinks(body, final)
	}
	origin := &url.URL{Scheme: u.Scheme, Host: u.Host}
	cands = append(cands, candidate{url: origin.ResolveReference(&url.URL{Path: "/apple-touch-icon.png"}).String(), score: 150},
		candidate{url: origin.ResolveReference(&url.URL{Path: "/favicon.ico"}).String(), score: 1})
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })

	seen := map[string]bool{}
	var best image.Image
	for _, c := range cands {
		if seen[c.url] {
			continue
		}
		seen[c.url] = true
		data, _, err := get(ctx, c.url, maxImage)
		if err != nil {
			continue
		}
		img, err := Decode(data)
		if err != nil || img.Bounds().Dx() < 16 {
			continue
		}
		if best == nil || img.Bounds().Dx() > best.Bounds().Dx() {
			best = img
		}
		if best.Bounds().Dx() >= 96 { // good enough for a tile
			break
		}
	}
	if best == nil {
		return nil, ErrNoIcon
	}
	return best, nil
}

func get(ctx context.Context, rawURL string, limit int64) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("%s: HTTP %d", rawURL, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return data, resp.Request.URL, err
}

// parseLinks collects <link rel="…icon…" href sizes> from a page, best first.
func parseLinks(page []byte, base *url.URL) []candidate {
	var out []candidate
	z := html.NewTokenizer(bytes.NewReader(page))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return out
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			if string(name) == "body" {
				return out // icons live in <head>
			}
			if string(name) != "link" || !hasAttr {
				continue
			}
			var rel, href, sizes, typ string
			for {
				k, v, more := z.TagAttr()
				switch string(k) {
				case "rel":
					rel = strings.ToLower(string(v))
				case "href":
					href = string(v)
				case "sizes":
					sizes = strings.ToLower(string(v))
				case "type":
					typ = strings.ToLower(string(v))
				}
				if !more {
					break
				}
			}
			if !strings.Contains(rel, "icon") || href == "" || strings.Contains(typ, "svg") || strings.HasSuffix(strings.ToLower(href), ".svg") {
				continue
			}
			ref, err := url.Parse(href)
			if err != nil {
				continue
			}
			score := 10 + largestSize(sizes)
			if strings.Contains(rel, "apple-touch-icon") {
				score += 200 // usually 180×180 and made for home screens
			}
			out = append(out, candidate{url: base.ResolveReference(ref).String(), score: score})
		}
	}
}

func largestSize(sizes string) int {
	best := 0
	for _, s := range strings.Fields(sizes) {
		w, _, ok := strings.Cut(s, "x")
		if n, err := strconv.Atoi(w); ok && err == nil && n > best {
			best = n
		}
	}
	return best
}

// Decode reads PNG, JPEG, GIF, WebP and ICO images.
func Decode(data []byte) (image.Image, error) {
	if len(data) >= 4 && data[0] == 0 && data[1] == 0 && data[2] == 1 && data[3] == 0 {
		return decodeICO(data)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}
