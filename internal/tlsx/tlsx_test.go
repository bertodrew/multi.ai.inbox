package tlsx

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePair(t *testing.T, dir string, c *tls.Certificate) {
	t.Helper()
	keyDER, err := x509.MarshalECPrivateKey(c.PrivateKey.(*ecdsa.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Certificate[0]})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(filepath.Join(dir, "mx.test.crt"), certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mx.test.key"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFallbackThenIssuedCertificate(t *testing.T) {
	root := t.TempDir()
	l := &Loader{CertGlob: filepath.Join(root, "*", "mx.test.crt"), KeyGlob: filepath.Join(root, "*", "mx.test.key"), Hostname: "mx.test"}

	fallback, err := l.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil || fallback == nil {
		t.Fatalf("no fallback certificate: %v", err)
	}

	issued, err := selfSigned("mx.test")
	if err != nil {
		t.Fatal(err)
	}
	writePair(t, filepath.Join(root, "acme-v02.api.letsencrypt.org-directory"), issued)
	l.checked = time.Time{} // force the periodic reload now

	got, err := l.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Certificate[0]) != string(issued.Certificate[0]) {
		t.Fatal("loader did not switch to the issued certificate")
	}
	if string(got.Certificate[0]) == string(fallback.Certificate[0]) {
		t.Fatal("still serving the fallback certificate")
	}
}
