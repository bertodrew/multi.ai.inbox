package blob

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeS3 is a path style S3 endpoint that checks the payload hash and the
// signature of every request, recomputed from what arrived on the wire.
type fakeS3 struct {
	t       *testing.T
	bucket  string
	signer  signer
	mu      sync.Mutex
	objects map[string][]byte
	fail    int  // status to answer every request with, when set
	quiet   bool // a signature mismatch is expected, do not fail the test
	url     string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	hash := r.Header.Get("X-Amz-Content-Sha256")
	if hash != sha256Hex(body) {
		f.t.Errorf("%s %s: x-amz-content-sha256 %q does not match the body", r.Method, r.URL.Path, hash)
		http.Error(w, "bad hash", http.StatusBadRequest)
		return
	}
	when, err := time.Parse(amzDateFormat, r.Header.Get("X-Amz-Date"))
	if err != nil {
		f.t.Errorf("x-amz-date: %v", err)
	}
	got := r.Header.Get("Authorization")
	// Re-sign only the headers the client signed (the transport adds others).
	_, rest, _ := strings.Cut(got, "SignedHeaders=")
	signedList, _, _ := strings.Cut(rest, ",")
	check, _ := http.NewRequest(r.Method, "http://"+r.Host+r.RequestURI, nil)
	check.Host = r.Host
	for _, h := range strings.Split(signedList, ";") {
		if h != "host" && h != "x-amz-date" {
			check.Header.Set(h, r.Header.Get(h))
		}
	}
	f.signer.sign(check, hash, when)
	if want := check.Header.Get("Authorization"); got != want {
		if !f.quiet {
			f.t.Errorf("%s %s: signature mismatch\n got %s\nwant %s", r.Method, r.RequestURI, got, want)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><Error><Code>SignatureDoesNotMatch</Code><Message>no</Message></Error>`)
		return
	}
	if !strings.Contains(got, "/"+f.signer.Region+"/s3/aws4_request") {
		f.t.Errorf("credential scope: %s", got)
	}
	if f.fail != 0 {
		w.WriteHeader(f.fail)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><Error><Code>InternalError</Code><Message>try again</Message></Error>`)
		return
	}
	prefix := "/" + f.bucket + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.Error(w, "no such bucket", http.StatusNotFound)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, prefix)
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case http.MethodPut:
		f.objects[key] = body
	case http.MethodGet:
		data, ok := f.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><Error><Code>NoSuchKey</Code></Error>`)
			return
		}
		_, _ = w.Write(data)
	case http.MethodDelete:
		if _, ok := f.objects[key]; !ok {
			// Real S3 answers 204 here; some compatible stores answer 404.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func newFakeS3(t *testing.T) (*S3, *fakeS3) {
	t.Helper()
	fake := &fakeS3{t: t, bucket: "mai-blobs", objects: map[string][]byte{},
		signer: signer{AccessKey: "AK", SecretKey: "SK", Region: "fsn1", Service: "s3"}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	fake.url = srv.URL
	store, err := NewS3(srv.URL, "fsn1", "mai-blobs", "AK", "SK")
	if err != nil {
		t.Fatal(err)
	}
	return store, fake
}

func TestS3RoundTrip(t *testing.T) {
	ctx := context.Background()
	store, fake := newFakeS3(t)
	var _ Store = store

	keys := []string{
		"raw/org-1/2026/10/inbox-abc.eml",
		"raw/org-1/2026/10/<id+1@mx.example.eu>.eml",
		"att/org-1/msg/report final (v2).pdf",
	}
	for i, key := range keys {
		data := bytes.Repeat([]byte{byte('a' + i)}, 1000*(i+1))
		if err := store.Put(ctx, key, data); err != nil {
			t.Fatalf("put %q: %v", key, err)
		}
		got, err := store.Get(ctx, key)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("get %q: %v (len %d)", key, err, len(got))
		}
		if _, ok := fake.objects[key]; !ok {
			t.Fatalf("object %q not stored under its path style key: %v", key, fake.objects)
		}
	}
	if err := store.Put(ctx, "empty", nil); err != nil {
		t.Fatalf("put empty: %v", err)
	}
	if got, err := store.Get(ctx, "empty"); err != nil || len(got) != 0 {
		t.Fatalf("get empty: %v %q", err, got)
	}

	if err := store.Delete(ctx, keys[0]); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Get(ctx, keys[0]); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("get after delete: %v, want fs.ErrNotExist", err)
	}
	// A retried deletion (404) is a success.
	if err := store.Delete(ctx, keys[0]); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
	if _, err := store.Get(ctx, "../etc/passwd"); err == nil {
		t.Fatal("key with .. accepted")
	}
}

func TestS3Errors(t *testing.T) {
	ctx := context.Background()
	store, fake := newFakeS3(t)
	fake.fail = http.StatusInternalServerError
	err := store.Put(ctx, "k", []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "500 InternalError") {
		t.Fatalf("put error: %v", err)
	}
	if err := store.Delete(ctx, "k"); err == nil {
		t.Fatal("delete with a 500 reported success")
	}

	// A wrong secret is refused by the server's signature check.
	fake.fail = 0
	wrong, _ := NewS3(fake.url, "fsn1", "mai-blobs", "AK", "other")
	fake.quiet = true
	if err := wrong.Put(ctx, "k", []byte("x")); err == nil || !strings.Contains(err.Error(), "SignatureDoesNotMatch") {
		t.Fatalf("wrong secret: %v", err)
	}
}

func TestNewS3Validation(t *testing.T) {
	for _, c := range [][5]string{
		{"", "fsn1", "b", "a", "s"},
		{"ftp://x", "fsn1", "b", "a", "s"},
		{"https://fsn1.your-objectstorage.com", "", "b", "a", "s"},
		{"https://fsn1.your-objectstorage.com", "fsn1", "", "a", "s"},
		{"https://fsn1.your-objectstorage.com", "fsn1", "a/b", "a", "s"},
		{"https://fsn1.your-objectstorage.com", "fsn1", "b", "", "s"},
		{"https://fsn1.your-objectstorage.com", "fsn1", "b", "a", ""},
	} {
		if _, err := NewS3(c[0], c[1], c[2], c[3], c[4]); err == nil {
			t.Errorf("NewS3(%q) accepted", c)
		}
	}
	s, err := NewS3("https://fsn1.your-objectstorage.com/", "fsn1", "mai", "a", "s")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := s.objectURL("raw/o/<a@b>.eml")
	if got := u.String(); got != "https://fsn1.your-objectstorage.com/mai/raw/o/%3Ca%40b%3E.eml" {
		t.Errorf("object URL %s", got)
	}
}
