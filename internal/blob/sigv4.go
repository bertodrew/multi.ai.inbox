package blob

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// AWS Signature Version 4, written against the standard library only
// (decision D8: no AWS SDK). Reference:
// https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_sigv-create-signed-request.html

const (
	sigAlgorithm  = "AWS4-HMAC-SHA256"
	amzDateFormat = "20060102T150405Z"
)

type signer struct {
	AccessKey string
	SecretKey string
	Region    string
	Service   string
}

// sign adds X-Amz-Date and Authorization to req. Every header present in
// req.Header at this point is signed, plus Host. payloadHash is the hex
// SHA-256 of the body; the caller also sends it as X-Amz-Content-Sha256
// when the service requires that (S3 does).
func (s signer) sign(req *http.Request, payloadHash string, now time.Time) {
	now = now.UTC()
	amzDate := now.Format(amzDateFormat)
	req.Header.Set("X-Amz-Date", amzDate)
	creq, signed := canonicalRequest(req, payloadHash)
	scope := now.Format("20060102") + "/" + s.Region + "/" + s.Service + "/aws4_request"
	sts := stringToSign(amzDate, scope, creq)

	key := hmacSHA256([]byte("AWS4"+s.SecretKey), now.Format("20060102"))
	key = hmacSHA256(key, s.Region)
	key = hmacSHA256(key, s.Service)
	key = hmacSHA256(key, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(key, sts))

	req.Header.Set("Authorization", sigAlgorithm+" Credential="+s.AccessKey+"/"+scope+
		", SignedHeaders="+signed+", Signature="+sig)
}

func stringToSign(amzDate, scope, canonicalRequest string) string {
	return sigAlgorithm + "\n" + amzDate + "\n" + scope + "\n" + sha256Hex([]byte(canonicalRequest))
}

// canonicalRequest returns the canonical request and the signed header list.
// The URI is used as escaped on the wire, without normalization, which is
// what S3 expects (it does not double encode).
func canonicalRequest(req *http.Request, payloadHash string) (string, string) {
	uri := req.URL.EscapedPath()
	if uri == "" {
		uri = "/"
	}
	headers := map[string][]string{}
	for k, vs := range req.Header {
		lk := strings.ToLower(k)
		if lk == "authorization" {
			continue
		}
		for _, v := range vs {
			headers[lk] = append(headers[lk], trimHeaderValue(v))
		}
	}
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	headers["host"] = []string{host}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var ch strings.Builder
	for _, k := range names {
		ch.WriteString(k + ":" + strings.Join(headers[k], ",") + "\n")
	}
	signed := strings.Join(names, ";")
	return req.Method + "\n" + uri + "\n" + canonicalQuery(req.URL.RawQuery) + "\n" + ch.String() + "\n" + signed + "\n" + payloadHash, signed
}

// trimHeaderValue removes surrounding spaces and collapses inner runs of
// spaces to one, as the canonical form requires.
func trimHeaderValue(v string) string {
	return strings.Join(strings.Fields(v), " ")
}

// canonicalQuery sorts parameters by name, then value, each URI encoded.
func canonicalQuery(raw string) string {
	if raw == "" {
		return ""
	}
	type pair struct{ k, v string }
	var pairs []pair
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		if dk, err := url.QueryUnescape(k); err == nil {
			k = dk
		}
		if dv, err := url.QueryUnescape(v); err == nil {
			v = dv
		}
		pairs = append(pairs, pair{uriEncode(k, true), uriEncode(v, true)})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k != pairs[j].k {
			return pairs[i].k < pairs[j].k
		}
		return pairs[i].v < pairs[j].v
	})
	out := make([]string, len(pairs))
	for i, p := range pairs {
		out[i] = p.k + "=" + p.v
	}
	return strings.Join(out, "&")
}

// uriEncode percent encodes every byte except the unreserved characters
// (A-Z a-z 0-9 - _ . ~), with upper case hex; "/" is kept unless
// encodeSlash is set (object key paths keep it, query values do not).
func uriEncode(s string, encodeSlash bool) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&15])
		}
	}
	return b.String()
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
