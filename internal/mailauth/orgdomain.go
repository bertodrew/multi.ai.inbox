// Package mailauth evaluates SPF (RFC 7208) and DMARC (RFC 7489) for inbound
// mail at the SMTP edge. DNS goes through the Resolver interface so tests can
// use fakes; net.DefaultResolver satisfies it in production.
package mailauth

import "strings"

// multiLabelSuffixes are common public suffixes with two labels. OrgDomain
// keeps one more label under them.
//
// Limitation: this is not the Public Suffix List. Domains under suffixes that
// are missing here (for example many regional suffixes, or private suffixes
// such as github.io) get an organizational domain that is one label too
// short, so relaxed alignment between two different registrants under such
// a suffix is possible. Exact (strict) alignment is unaffected.
var multiLabelSuffixes = map[string]bool{
	"co.uk": true, "org.uk": true, "me.uk": true, "ltd.uk": true, "plc.uk": true, "net.uk": true, "ac.uk": true, "gov.uk": true, "sch.uk": true,
	"com.au": true, "net.au": true, "org.au": true, "edu.au": true, "gov.au": true, "id.au": true,
	"co.nz": true, "net.nz": true, "org.nz": true, "govt.nz": true, "ac.nz": true,
	"co.jp": true, "ne.jp": true, "or.jp": true, "ac.jp": true, "go.jp": true,
	"co.kr": true, "or.kr": true, "co.in": true, "net.in": true, "org.in": true, "co.za": true, "org.za": true,
	"co.il": true, "org.il": true, "ac.il": true, "co.id": true, "or.id": true, "co.th": true, "in.th": true,
	"com.br": true, "net.br": true, "org.br": true, "com.mx": true, "com.ar": true, "com.co": true, "com.pe": true,
	"com.cn": true, "net.cn": true, "org.cn": true, "com.hk": true, "com.tw": true, "com.sg": true, "com.my": true,
	"com.tr": true, "com.ua": true, "com.pl": true, "net.pl": true, "org.pl": true, "co.at": true, "or.at": true,
	"com.es": true, "com.pt": true, "com.gr": true, "com.cy": true, "com.mt": true, "co.hu": true,
}

// OrgDomain returns the organizational domain (RFC 7489 section 3.2) with a
// simple heuristic: the last two labels, or the last three when the last two
// form a known multi label public suffix. See multiLabelSuffixes.
func OrgDomain(domain string) string {
	d := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	labels := strings.Split(d, ".")
	if len(labels) <= 2 {
		return d
	}
	n := 2
	if multiLabelSuffixes[strings.Join(labels[len(labels)-2:], ".")] {
		n = 3
	}
	return strings.Join(labels[len(labels)-n:], ".")
}
