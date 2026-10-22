// Package tlsx serves a certificate that another process renews (Caddy
// obtains it from Let's Encrypt), reloading it from disk when it changes. Until
// the files exist it serves a self signed certificate, which opportunistic
// SMTP senders accept, so STARTTLS is always available.
package tlsx

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Loader struct {
	CertGlob string // may contain * (Caddy stores certificates per issuer directory)
	KeyGlob  string
	Hostname string

	mu       sync.Mutex
	cert     *tls.Certificate
	modTime  time.Time
	checked  time.Time
	fallback *tls.Certificate
}

// Config returns a TLS server configuration backed by the loader.
func (l *Loader) Config() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: l.GetCertificate}
}

func (l *Loader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Since(l.checked) > time.Minute || l.cert == nil {
		l.checked = time.Now()
		l.reload()
	}
	if l.cert != nil {
		return l.cert, nil
	}
	if l.fallback == nil {
		c, err := selfSigned(l.Hostname)
		if err != nil {
			return nil, err
		}
		l.fallback = c
	}
	return l.fallback, nil
}

// reload picks the newest matching certificate and loads it if it changed.
func (l *Loader) reload() {
	if l.CertGlob == "" || l.KeyGlob == "" {
		return
	}
	certPath, certMod := newest(l.CertGlob)
	keyPath, _ := newest(l.KeyGlob)
	if certPath == "" || keyPath == "" || (l.cert != nil && !certMod.After(l.modTime)) {
		return
	}
	c, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return // keep serving the previous or fallback certificate
	}
	l.cert, l.modTime = &c, certMod
}

func newest(pattern string) (string, time.Time) {
	matches, _ := filepath.Glob(pattern)
	var best string
	var bestMod time.Time
	for _, m := range matches {
		if st, err := os.Stat(m); err == nil && st.ModTime().After(bestMod) {
			best, bestMod = m, st.ModTime()
		}
	}
	return best, bestMod
}

func selfSigned(host string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tmpl := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(90 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("self signed certificate: %w", err)
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}
