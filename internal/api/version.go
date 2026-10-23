package api

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// The REST surface answers on two path prefixes. v1 is the documented version
// from now on. v0 is not a fork: the same handler answers both, the version is
// read from the path and carried in the request context, and the response
// headers say which version answered. Nothing in a body changes between them.
const (
	VersionV0 = "v0"
	VersionV1 = "v1"
)

// CurrentVersion is the version new clients should use. The SDKs default to
// it and it is what Aimulti-Api-Version names on every response.
const CurrentVersion = VersionV1

// SunsetV0 is the last day /v0 is promised to answer. It sits more than a year
// after v1 shipped, and the deprecation policy (docs page "Versions and SDKs")
// promises at least 12 months of notice before any sunset date moves closer.
// The Sunset header is sent as an HTTP date built from this value.
var SunsetV0 = time.Date(2027, time.December, 31, 0, 0, 0, 0, time.UTC)

// Response headers of the version contract.
const (
	// HeaderAPIVersion names the version that answered ("v0" or "v1").
	HeaderAPIVersion = "Aimulti-Api-Version"
	headerSunset     = "Sunset"
	headerLink       = "Link"
)

// versionKey carries the API version through the request context.
type versionKey struct{}

// VersionFrom reports the API version a request was made against: "v1", "v0",
// or "" for the few routes outside the versioned surface (/healthz, /mcp).
func VersionFrom(ctx context.Context) string {
	v, _ := ctx.Value(versionKey{}).(string)
	return v
}

// versionOf returns the version named by a path prefix, or "" when the path is
// not part of the versioned REST surface.
func versionOf(path string) string {
	for _, v := range []string{VersionV1, VersionV0} {
		if path == "/"+v || strings.HasPrefix(path, "/"+v+"/") {
			return v
		}
	}
	return ""
}

// successorPath maps a /v0 path to the same operation under /v1.
func successorPath(path string) string {
	if rest, ok := strings.CutPrefix(path, "/v0"); ok {
		return "/v1" + rest
	}
	return path
}

// versioned tags each request with its API version and writes the headers that
// make the contract visible: Aimulti-Api-Version on every versioned response,
// plus Sunset and Link rel="successor-version" on /v0.
func (s *Server) versioned(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := versionOf(r.URL.Path)
		if v == "" {
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Set(HeaderAPIVersion, v)
		if v == VersionV0 {
			// EscapedPath keeps the encoding the client sent, so the link stays
			// a valid URI reference even for a Message-ID in angle brackets.
			h.Set(headerSunset, SunsetV0.Format(http.TimeFormat))
			h.Set(headerLink, "<"+successorPath(r.URL.EscapedPath())+">; rel=\"successor-version\"")
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), versionKey{}, v)))
	})
}

// aliases returns a route pattern at both supported versions. One handler is
// registered per operation, so /v1 can never drift from /v0.
func aliases(pattern string) []string {
	prefix, path := "", pattern
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		prefix, path = pattern[:i+1], pattern[i+1:]
	}
	rest, ok := strings.CutPrefix(path, "/v0/")
	if !ok {
		return []string{pattern}
	}
	return []string{pattern, prefix + "/v1/" + rest}
}
