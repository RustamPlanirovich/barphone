package tlscert

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCertificateIsCreatedOnceAndKept(t *testing.T) {
	dir := t.TempDir()
	cert, fp, err := LoadOrCreate(dir, "ПК")
	if err != nil {
		t.Fatal(err)
	}
	if len(fp) != 43 {
		t.Fatalf("fingerprint %q: want 43 base64url characters", fp)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil || leaf.NotAfter.Year()-leaf.NotBefore.Year() < 19 {
		t.Fatalf("certificate: %v %v", leaf, err)
	}
	_, again, err := LoadOrCreate(dir, "ПК")
	if err != nil || again != fp {
		t.Fatalf("second start must keep the certificate: %s vs %s (%v)", again, fp, err)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(filepath.Join(dir, keyFile)); st.Mode().Perm() != 0o600 {
			t.Fatalf("key permissions: %v", st.Mode())
		}
	}
	os.WriteFile(filepath.Join(dir, keyFile), []byte("damaged"), 0o600)
	if _, _, err := LoadOrCreate(dir, "ПК"); err == nil {
		t.Fatal("a damaged key must be an error, not a silent new certificate")
	}
}
