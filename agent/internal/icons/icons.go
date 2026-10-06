// Package icons stores button icons as normalized 256x256 PNG files addressed by content hash.
package icons

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"

	_ "image/gif"
	_ "image/jpeg"

	"golang.org/x/image/draw"
)

const Size = 256

var hashRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

func ValidHash(h string) bool { return hashRe.MatchString(h) }

type Store struct{ dir string }

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Path(hash string) (string, bool) {
	if !ValidHash(hash) {
		return "", false
	}
	p := filepath.Join(s.dir, hash+".png")
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	return p, true
}

func (s *Store) Has(hash string) bool {
	_, ok := s.Path(hash)
	return ok
}

// PutImage normalizes img to Size x Size and stores it, returning its hash.
func (s *Store) PutImage(img image.Image) (string, error) {
	data, err := Normalize(img)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])[:16]
	p := filepath.Join(s.dir, hash+".png")
	if _, err := os.Stat(p); err == nil {
		return hash, nil
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	return hash, os.Rename(tmp, p)
}

// PutEncoded decodes PNG/JPEG/GIF bytes and stores them via PutImage.
func (s *Store) PutEncoded(data []byte) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("decode icon: %w", err)
	}
	return s.PutImage(img)
}

// Normalize fits img into a transparent Size x Size square and encodes it as PNG.
func Normalize(img image.Image) ([]byte, error) {
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil, fmt.Errorf("empty image")
	}
	dst := image.NewNRGBA(image.Rect(0, 0, Size, Size))
	w, h := Size, Size
	if b.Dx() > b.Dy() {
		h = Size * b.Dy() / b.Dx()
	} else if b.Dy() > b.Dx() {
		w = Size * b.Dx() / b.Dy()
	}
	r := image.Rect((Size-w)/2, (Size-h)/2, (Size-w)/2+w, (Size-h)/2+h)
	if b.Dx() == w && b.Dy() == h {
		draw.Draw(dst, r, img, b.Min, draw.Src)
	} else {
		draw.CatmullRom.Scale(dst, r, img, b, draw.Src, nil)
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
