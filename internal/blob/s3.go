package blob

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// S3 stores blobs in an S3 compatible bucket (Hetzner Object Storage) with
// path style URLs (endpoint/bucket/key) and Signature Version 4. Every
// request signs the SHA-256 of its payload; UNSIGNED-PAYLOAD is never used.
type S3 struct {
	endpoint *url.URL
	bucket   string
	signer   signer
	// Client sends the requests; nil means a client with a 60 second timeout.
	Client *http.Client
	// Now is the signing clock; nil means time.Now (tests pin it).
	Now func() time.Time
}

// NewS3 validates the settings: endpoint is the base URL, for example
// https://fsn1.your-objectstorage.com, and region its location (fsn1).
func NewS3(endpoint, region, bucket, accessKey, secretKey string) (*S3, error) {
	u, err := url.Parse(strings.TrimSuffix(endpoint, "/"))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("s3 endpoint must be an http(s) URL, got %q", endpoint)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("s3 endpoint must not have a query or fragment")
	}
	switch {
	case region == "":
		return nil, errors.New("s3 region is required")
	case bucket == "" || strings.ContainsAny(bucket, "/?#% "):
		return nil, fmt.Errorf("invalid s3 bucket %q", bucket)
	case accessKey == "" || secretKey == "":
		return nil, errors.New("s3 access key and secret key are required")
	}
	return &S3{endpoint: u, bucket: bucket, signer: signer{AccessKey: accessKey, SecretKey: secretKey, Region: region, Service: "s3"}}, nil
}

func (s *S3) objectURL(key string) (*url.URL, error) {
	key = strings.TrimLeft(key, "/")
	if key == "" || strings.Contains(key, "..") {
		return nil, errors.New("invalid blob key")
	}
	u := *s.endpoint
	u.Path = s.endpoint.Path + "/" + s.bucket + "/" + key
	u.RawPath = s.endpoint.EscapedPath() + "/" + uriEncode(s.bucket, true) + "/" + uriEncode(key, false)
	return &u, nil
}

func (s *S3) do(ctx context.Context, method, key string, body []byte) (*http.Response, error) {
	u, err := s.objectURL(key)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		req.Body, req.ContentLength = http.NoBody, 0
	}
	hash := sha256Hex(body)
	req.Header.Set("X-Amz-Content-Sha256", hash)
	if method == http.MethodPut {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	s.signer.sign(req, hash, now())
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return client.Do(req)
}

func (s *S3) Put(ctx context.Context, key string, data []byte) error {
	resp, err := s.do(ctx, http.MethodPut, key, data)
	if err != nil {
		return fmt.Errorf("blob put: %w", err)
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusOK {
		return s3Error("put", key, resp)
	}
	return nil
}

func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	resp, err := s.do(ctx, http.MethodGet, key, nil)
	if err != nil {
		return nil, fmt.Errorf("blob get: %w", err)
	}
	defer drain(resp)
	switch resp.StatusCode {
	case http.StatusOK:
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("blob get %s: %w", key, err)
		}
		return data, nil
	case http.StatusNotFound:
		return nil, fmt.Errorf("blob get %s: %w", key, fs.ErrNotExist)
	default:
		return nil, s3Error("get", key, resp)
	}
}

// Delete removes an object; a missing one (404) counts as deleted, like FS.
func (s *S3) Delete(ctx context.Context, key string) error {
	resp, err := s.do(ctx, http.MethodDelete, key, nil)
	if err != nil {
		return fmt.Errorf("blob delete: %w", err)
	}
	defer drain(resp)
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	default:
		return s3Error("delete", key, resp)
	}
}

// s3Error reports the status and the S3 error code (never the request).
func s3Error(op, key string, resp *http.Response) error {
	var body struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	_ = xml.Unmarshal(raw, &body)
	if body.Code == "" {
		return fmt.Errorf("blob %s %s: s3 returned %d", op, key, resp.StatusCode)
	}
	return fmt.Errorf("blob %s %s: s3 returned %d %s: %s", op, key, resp.StatusCode, body.Code, body.Message)
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
}
