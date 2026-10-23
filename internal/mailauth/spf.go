package mailauth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Resolver is the DNS subset used by SPF, DKIM and DMARC. net.Resolver
// satisfies it. A missing name or record must be reported as a *net.DNSError
// with IsNotFound set; any other error counts as a temporary DNS failure.
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

// SPFResult is an RFC 7208 section 2.6 result.
type SPFResult string

const (
	SPFPass      SPFResult = "pass"
	SPFFail      SPFResult = "fail"
	SPFSoftFail  SPFResult = "softfail"
	SPFNeutral   SPFResult = "neutral"
	SPFNone      SPFResult = "none"
	SPFTempError SPFResult = "temperror"
	SPFPermError SPFResult = "permerror"
)

// Processing limits from RFC 7208 section 4.6.4.
const (
	maxDNSLookups  = 10 // include, a, mx, ptr, exists and redirect
	maxVoidLookups = 2  // lookups that return no records
	maxMXNames     = 10 // MX hosts looked up per mx mechanism
)

// CheckSPF evaluates the SPF policy for a client IP and the MAIL FROM
// identity (RFC 7208 check_host). With a null reverse path the identity is
// postmaster@helo, as section 2.4 requires. The HELO identity is not checked
// separately. The ptr mechanism is deprecated (section 5.5): it counts as a
// DNS lookup but never matches.
func CheckSPF(ctx context.Context, r Resolver, ip net.IP, mailFrom, helo string) SPFResult {
	if ip == nil {
		return SPFNone
	}
	sender := strings.TrimSpace(mailFrom)
	if sender == "" {
		sender = "postmaster@" + helo
	}
	local, domain, ok := strings.Cut(sender, "@")
	if !ok {
		local, domain = "postmaster", sender
	}
	if local == "" {
		local = "postmaster"
	}
	e := &spfEval{ctx: ctx, r: r, ip: ip, sender: local + "@" + domain, local: local, senderDomain: domain, helo: helo}
	return e.check(domain)
}

type spfEval struct {
	ctx          context.Context
	r            Resolver
	ip           net.IP
	sender       string
	local        string
	senderDomain string
	helo         string
	lookups      int
	voids        int
}

// errPerm and errTemp end evaluation with permerror and temperror.
var (
	errPerm = errors.New("spf permerror")
	errTemp = errors.New("spf temperror")
)

type directive struct {
	qual  byte // + - ~ ?
	name  string
	spec  string // domain-spec (a, mx, include, exists, ptr)
	cidr4 int
	cidr6 int
	net   *net.IPNet // ip4, ip6
}

func (e *spfEval) check(domain string) SPFResult {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	if !validDomain(domain) {
		return SPFNone
	}
	txts, err := e.r.LookupTXT(e.ctx, domain)
	if err != nil {
		if notFound(err) {
			return SPFNone
		}
		return SPFTempError
	}
	var record string
	n := 0
	for _, t := range txts {
		if isSPFRecord(t) {
			record, n = t, n+1
		}
	}
	switch {
	case n == 0:
		return SPFNone
	case n > 1:
		return SPFPermError
	}
	dirs, redirect, err := parseSPF(record)
	if err != nil {
		return SPFPermError
	}
	for _, d := range dirs {
		match, err := e.matches(d, domain)
		switch {
		case errors.Is(err, errTemp):
			return SPFTempError
		case err != nil:
			return SPFPermError
		case match:
			return qualifierResult(d.qual)
		}
	}
	if redirect != "" {
		if err := e.countLookup(); err != nil {
			return SPFPermError
		}
		target, err := e.expand(redirect, domain)
		if err != nil {
			return SPFPermError
		}
		res := e.check(target)
		if res == SPFNone {
			return SPFPermError // section 6.1
		}
		return res
	}
	return SPFNeutral
}

func isSPFRecord(t string) bool {
	return strings.EqualFold(t, "v=spf1") || (len(t) > 7 && strings.EqualFold(t[:7], "v=spf1 "))
}

func qualifierResult(q byte) SPFResult {
	switch q {
	case '-':
		return SPFFail
	case '~':
		return SPFSoftFail
	case '?':
		return SPFNeutral
	}
	return SPFPass
}

var (
	modifierRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9._-]*)=(.*)$`)
	cidrRe     = regexp.MustCompile(`^(.*?)(?:/([0-9]+))?(?://([0-9]+))?$`)
)

// parseSPF checks the whole record before evaluation: any syntax error is a
// permerror (section 4.6).
func parseSPF(record string) ([]directive, string, error) {
	var dirs []directive
	redirect, seenRedirect, seenExp := "", false, false
	for _, term := range strings.Fields(record)[1:] {
		if m := modifierRe.FindStringSubmatch(term); m != nil {
			switch strings.ToLower(m[1]) {
			case "redirect":
				if seenRedirect || m[2] == "" {
					return nil, "", errPerm
				}
				seenRedirect, redirect = true, m[2]
			case "exp":
				if seenExp {
					return nil, "", errPerm
				}
				seenExp = true // explanations are not used
			}
			continue // unknown modifiers are ignored (section 6)
		}
		d := directive{qual: '+', cidr4: 32, cidr6: 128}
		if strings.ContainsRune("+-~?", rune(term[0])) {
			d.qual, term = term[0], term[1:]
		}
		name, arg := term, ""
		if i := strings.IndexAny(term, ":/"); i >= 0 {
			name, arg = term[:i], term[i:]
		}
		d.name = strings.ToLower(name)
		switch d.name {
		case "all":
			if arg != "" {
				return nil, "", errPerm
			}
		case "include", "exists":
			if !strings.HasPrefix(arg, ":") || len(arg) < 2 {
				return nil, "", errPerm
			}
			d.spec = arg[1:]
		case "ptr":
			if arg != "" {
				if !strings.HasPrefix(arg, ":") || len(arg) < 2 {
					return nil, "", errPerm
				}
				d.spec = arg[1:]
			}
		case "a", "mx":
			m := cidrRe.FindStringSubmatch(arg)
			if m == nil {
				return nil, "", errPerm
			}
			if m[1] != "" {
				if !strings.HasPrefix(m[1], ":") || len(m[1]) < 2 {
					return nil, "", errPerm
				}
				d.spec = m[1][1:]
			}
			var err error
			if d.cidr4, err = cidrLen(m[2], 32); err != nil {
				return nil, "", err
			}
			if d.cidr6, err = cidrLen(m[3], 128); err != nil {
				return nil, "", err
			}
		case "ip4", "ip6":
			if !strings.HasPrefix(arg, ":") {
				return nil, "", errPerm
			}
			addr, bits, hasBits := strings.Cut(arg[1:], "/")
			ip := net.ParseIP(addr)
			if ip == nil || (d.name == "ip4") != (ip.To4() != nil && !strings.Contains(addr, ":")) {
				return nil, "", errPerm
			}
			size := 32
			if d.name == "ip6" {
				size = 128
			} else {
				ip = ip.To4()
			}
			ones := size
			if hasBits {
				n, err := cidrLen(bits, size)
				if err != nil || bits == "" {
					return nil, "", errPerm
				}
				ones = n
			}
			d.net = &net.IPNet{IP: ip.Mask(net.CIDRMask(ones, size)), Mask: net.CIDRMask(ones, size)}
		default:
			return nil, "", errPerm
		}
		dirs = append(dirs, d)
	}
	return dirs, redirect, nil
}

func cidrLen(s string, max int) (int, error) {
	if s == "" {
		return max, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > max {
		return 0, errPerm
	}
	return n, nil
}

func (e *spfEval) countLookup() error {
	e.lookups++
	if e.lookups > maxDNSLookups {
		return errPerm
	}
	return nil
}

func (e *spfEval) countVoid() error {
	e.voids++
	if e.voids > maxVoidLookups {
		return errPerm
	}
	return nil
}

func (e *spfEval) matches(d directive, domain string) (bool, error) {
	if d.name == "all" {
		return true, nil
	}
	if d.name == "ip4" || d.name == "ip6" {
		return d.net.Contains(e.ip), nil
	}
	if err := e.countLookup(); err != nil {
		return false, err
	}
	target := domain
	if d.spec != "" {
		var err error
		if target, err = e.expand(d.spec, domain); err != nil {
			return false, err
		}
	}
	switch d.name {
	case "include":
		switch e.check(target) {
		case SPFPass:
			return true, nil
		case SPFTempError:
			return false, errTemp
		case SPFPermError, SPFNone:
			return false, errPerm
		}
		return false, nil
	case "a":
		ips, err := e.addrs(target)
		if err != nil {
			return false, err
		}
		return e.inAny(ips, d), nil
	case "mx":
		mxs, err := e.r.LookupMX(e.ctx, target)
		if err != nil {
			if notFound(err) {
				return false, e.countVoid()
			}
			return false, errTemp
		}
		if len(mxs) > maxMXNames {
			return false, errPerm
		}
		for _, mx := range mxs {
			host := strings.TrimSuffix(mx.Host, ".")
			if host == "" {
				continue // null MX (RFC 7505)
			}
			ips, err := e.addrs(host)
			if err != nil {
				return false, err
			}
			if e.inAny(ips, d) {
				return true, nil
			}
		}
		return false, nil
	case "exists":
		ips, err := e.r.LookupIP(e.ctx, "ip4", target)
		if err != nil {
			if notFound(err) {
				return false, e.countVoid()
			}
			return false, errTemp
		}
		return len(ips) > 0, nil
	}
	return false, nil // ptr: deprecated, never matches
}

// addrs looks up the addresses of host in the client's address family.
func (e *spfEval) addrs(host string) ([]net.IP, error) {
	network := "ip6"
	if e.ip.To4() != nil {
		network = "ip4"
	}
	ips, err := e.r.LookupIP(e.ctx, network, host)
	if err != nil {
		if notFound(err) {
			return nil, e.countVoid()
		}
		return nil, errTemp
	}
	if len(ips) == 0 {
		return nil, e.countVoid()
	}
	return ips, nil
}

func (e *spfEval) inAny(ips []net.IP, d directive) bool {
	v4 := e.ip.To4() != nil
	for _, ip := range ips {
		var n *net.IPNet
		if v4 {
			if ip.To4() == nil {
				continue
			}
			n = &net.IPNet{IP: ip.To4(), Mask: net.CIDRMask(d.cidr4, 32)}
		} else {
			if ip.To4() != nil {
				continue
			}
			n = &net.IPNet{IP: ip.To16(), Mask: net.CIDRMask(d.cidr6, 128)}
		}
		n.IP = n.IP.Mask(n.Mask)
		if n.Contains(e.ip) {
			return true
		}
	}
	return false
}

// expand applies macro expansion (section 7) to a domain-spec. The c, r and
// t macros are only allowed in explanations, which are not evaluated.
func (e *spfEval) expand(spec, domain string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(spec); i++ {
		c := spec[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		if i+1 >= len(spec) {
			return "", errPerm
		}
		i++
		switch spec[i] {
		case '%':
			b.WriteByte('%')
		case '_':
			b.WriteByte(' ')
		case '-':
			b.WriteString("%20")
		case '{':
			end := strings.IndexByte(spec[i:], '}')
			if end < 2 {
				return "", errPerm
			}
			v, err := e.macro(spec[i+1:i+end], domain)
			if err != nil {
				return "", err
			}
			b.WriteString(v)
			i += end
		default:
			return "", errPerm
		}
	}
	out := strings.TrimSuffix(b.String(), ".")
	// Section 7.3: drop leading labels until the name fits in 253 octets.
	for len(out) > 253 {
		_, rest, ok := strings.Cut(out, ".")
		if !ok {
			return "", errPerm
		}
		out = rest
	}
	return out, nil
}

var macroRe = regexp.MustCompile(`^([a-zA-Z])([0-9]*)(r?)([.+,/_=-]*)$`)

func (e *spfEval) macro(body, domain string) (string, error) {
	m := macroRe.FindStringSubmatch(strings.Replace(body, "R", "r", 1))
	if m == nil {
		return "", errPerm
	}
	letter := m[1]
	var v string
	switch strings.ToLower(letter) {
	case "s":
		v = e.sender
	case "l":
		v = e.local
	case "o":
		v = e.senderDomain
	case "d":
		v = domain
	case "i":
		v = macroIP(e.ip)
	case "h":
		v = e.helo
	case "v":
		v = "ip6"
		if e.ip.To4() != nil {
			v = "in-addr"
		}
	case "p":
		v = "unknown"
	default:
		return "", errPerm
	}
	delims := m[4]
	if delims == "" {
		delims = "."
	}
	parts := strings.FieldsFunc(v, func(r rune) bool { return strings.ContainsRune(delims, r) })
	if m[3] == "r" {
		for a, z := 0, len(parts)-1; a < z; a, z = a+1, z-1 {
			parts[a], parts[z] = parts[z], parts[a]
		}
	}
	if m[2] != "" {
		n, err := strconv.Atoi(m[2])
		if err != nil || n == 0 {
			return "", errPerm
		}
		if n < len(parts) {
			parts = parts[len(parts)-n:]
		}
	}
	out := strings.Join(parts, ".")
	if letter != strings.ToLower(letter) {
		out = url.QueryEscape(out)
	}
	return out, nil
}

// macroIP formats %{i}: dotted quad for IPv4, dot separated nibbles for IPv6.
func macroIP(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	var b strings.Builder
	for i, by := range ip.To16() {
		if i > 0 {
			b.WriteByte('.')
		}
		fmt.Fprintf(&b, "%x.%x", by>>4, by&0xf)
	}
	return b.String()
}

func validDomain(d string) bool {
	if d == "" || len(d) > 253 || !strings.Contains(d, ".") {
		return false
	}
	for _, l := range strings.Split(d, ".") {
		if l == "" || len(l) > 63 {
			return false
		}
	}
	return true
}

func notFound(err error) bool {
	var de *net.DNSError
	return errors.As(err, &de) && de.IsNotFound
}
