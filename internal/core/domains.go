package core

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type DNSRecord struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	Status string `json:"status,omitempty"` // verified | missing
}

type Domain struct {
	ID            string      `json:"domain_id"`
	Name          string      `json:"domain"`
	Shared        bool        `json:"shared"`
	Status        string      `json:"status"` // verified | pending
	Records       []DNSRecord `json:"records"`
	LastCheckedAt *time.Time  `json:"last_checked_at,omitempty"`
	VerifiedAt    *time.Time  `json:"verified_at,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
}

// Resolver is the DNS subset used for verification (net.Resolver satisfies it).
type Resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

func (s *Service) resolver() Resolver {
	if s.DNS != nil {
		return s.DNS
	}
	return net.DefaultResolver
}

// AddDomain registers a domain with a fresh DKIM key. orgID empty means a
// shared platform domain, usable by every organization and verified by fiat.
func (s *Service) AddDomain(ctx context.Context, orgID, name string) (*Domain, error) {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	if !domainRe.MatchString(name) {
		return nil, Validation("invalid domain %q", name)
	}
	if name == s.SharedDomain && orgID != "" {
		return nil, errf(http.StatusConflict, "already_exists", "domain %s is the shared platform domain", name)
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("dkim key: %w", err)
	}
	privPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}))
	var org any
	if orgID != "" {
		org = orgID
	}
	err = s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `insert into domains (org_id, name, dkim_private_key, verified_at)
			values ($1, $2, $3, case when $1::uuid is null then now() end)`, org, name, privPEM)
		if isUniqueViolation(err) {
			return errf(http.StatusConflict, "already_exists", "domain %s already exists", name)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if orgID == "" {
		return s.domainSystem(ctx, name)
	}
	return s.GetDomain(ctx, orgID, name)
}

const domainCols = `id, name, org_id is null, dkim_selector, coalesce(dkim_private_key, ''), mx_ok, spf_ok, dkim_ok, dmarc_ok,
	last_checked_at, verified_at, created_at`

func (s *Service) scanDomain(row pgx.Row) (*Domain, error) {
	d := &Domain{}
	var sel, key string
	var mxOK, spfOK, dkimOK, dmarcOK bool
	if err := row.Scan(&d.ID, &d.Name, &d.Shared, &sel, &key, &mxOK, &spfOK, &dkimOK, &dmarcOK,
		&d.LastCheckedAt, &d.VerifiedAt, &d.CreatedAt); err != nil {
		return nil, err
	}
	d.Status = "pending"
	if d.VerifiedAt != nil {
		d.Status = "verified"
	}
	pub, err := dkimPublicKey(key)
	if err != nil {
		return nil, err
	}
	state := func(ok bool) string {
		if ok || d.Shared {
			return "verified"
		}
		return "missing"
	}
	d.Records = []DNSRecord{
		{Type: "MX", Name: d.Name, Value: "10 " + s.Hostname + ".", Status: state(mxOK)},
		{Type: "TXT", Name: d.Name, Value: s.spfValue(), Status: state(spfOK)},
		{Type: "TXT", Name: sel + "._domainkey." + d.Name, Value: "v=DKIM1; k=rsa; p=" + pub, Status: state(dkimOK)},
		{Type: "TXT", Name: "_dmarc." + d.Name, Value: "v=DMARC1; p=quarantine; rua=mailto:dmarc@" + d.Name, Status: state(dmarcOK)},
	}
	return d, nil
}

func (s *Service) spfValue() string {
	if s.SPFInclude != "" {
		return "v=spf1 mx include:" + s.SPFInclude + " -all"
	}
	return "v=spf1 mx -all"
}

func dkimPublicKey(privPEM string) (string, error) {
	block, _ := pem.Decode([]byte(privPEM))
	if block == nil {
		return "", errors.New("domain has no DKIM key")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pub), nil
}

func (s *Service) domainSystem(ctx context.Context, name string) (*Domain, error) {
	var out *Domain
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.scanDomain(tx.QueryRow(ctx, "select "+domainCols+" from domains where name = $1", name))
		return err
	})
	return out, err
}

// CreateDomain adds a customer domain owned by orgID.
func (s *Service) CreateDomain(ctx context.Context, orgID, name string) (*Domain, error) {
	plan, err := s.plan(ctx, orgID)
	if err != nil {
		return nil, err
	}
	var n int
	if err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select count(*) from domains where org_id = $1", orgID).Scan(&n)
	}); err != nil {
		return nil, err
	}
	if n >= plan.Domains {
		return nil, errf(http.StatusForbidden, "limit_exceeded", "the %s plan allows %d custom domains", plan.Name, plan.Domains)
	}
	return s.AddDomain(ctx, orgID, name)
}

// ListDomains returns the organization's domains and the shared ones.
func (s *Service) ListDomains(ctx context.Context, orgID string) ([]*Domain, error) {
	var list []*Domain
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select "+domainCols+" from domains order by org_id is null desc, created_at")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			d, err := s.scanDomain(rows)
			if err != nil {
				return err
			}
			list = append(list, d)
		}
		return rows.Err()
	})
	return list, err
}

func (s *Service) GetDomain(ctx context.Context, orgID, name string) (*Domain, error) {
	var out *Domain
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		var err error
		out, err = s.scanDomain(tx.QueryRow(ctx, "select "+domainCols+" from domains where name = $1", strings.ToLower(name)))
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound("domain")
		}
		return err
	})
	return out, err
}

func (s *Service) DeleteDomain(ctx context.Context, orgID, name string) error {
	return s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, "select id from domains where name = $1 and org_id = $2", strings.ToLower(name), orgID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound("domain")
		}
		if err != nil {
			return err
		}
		var inboxes int
		if err := tx.QueryRow(ctx, "select count(*) from inboxes where domain_id = $1", id).Scan(&inboxes); err != nil {
			return err
		}
		if inboxes > 0 {
			return errf(http.StatusConflict, "cannot_delete", "domain still has %d inboxes", inboxes)
		}
		_, err = tx.Exec(ctx, "delete from domains where id = $1", id)
		return err
	})
}

// VerifyDomain checks the DNS records of one org domain now.
func (s *Service) VerifyDomain(ctx context.Context, orgID, name string) (*Domain, error) {
	if _, err := s.GetDomain(ctx, orgID, name); err != nil {
		return nil, err
	}
	if err := s.checkDomain(ctx, strings.ToLower(name)); err != nil {
		return nil, err
	}
	return s.GetDomain(ctx, orgID, name)
}

// VerifyPendingDomains is the scheduler job: re-check unverified domains
// created in the last 7 days, at most once every 5 minutes each.
func (s *Service) VerifyPendingDomains(ctx context.Context) (int, error) {
	var names []string
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select name from domains where verified_at is null and org_id is not null
			and created_at > now() - interval '7 days'
			and (last_checked_at is null or last_checked_at < now() - interval '5 minutes') order by created_at limit 50`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				return err
			}
			names = append(names, n)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	for _, n := range names {
		if err := s.checkDomain(ctx, n); err != nil {
			return 0, err
		}
	}
	return len(names), nil
}

func (s *Service) checkDomain(ctx context.Context, name string) error {
	d, err := s.domainSystem(ctx, name)
	if err != nil {
		return err
	}
	if d.Shared {
		return nil // platform domains are verified by the operator
	}
	dns := s.resolver()
	lookupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	mxOK := false
	if mxs, err := dns.LookupMX(lookupCtx, name); err == nil {
		for _, mx := range mxs {
			if strings.EqualFold(strings.TrimSuffix(mx.Host, "."), s.Hostname) {
				mxOK = true
			}
		}
	}
	hasTXT := func(host string, ok func(string) bool) bool {
		txts, err := dns.LookupTXT(lookupCtx, host)
		if err != nil {
			return false
		}
		for _, t := range txts {
			if ok(t) {
				return true
			}
		}
		return false
	}
	spfOK := hasTXT(name, func(t string) bool {
		return strings.HasPrefix(t, "v=spf1") && (strings.Contains(t, " mx") || strings.Contains(t, s.Hostname))
	})
	dkimWant := strings.TrimPrefix(d.Records[2].Value, "v=DKIM1; k=rsa; ")
	dkimOK := hasTXT(d.Records[2].Name, func(t string) bool {
		return strings.Contains(strings.ReplaceAll(t, " ", ""), strings.ReplaceAll(dkimWant, " ", ""))
	})
	dmarcOK := hasTXT("_dmarc."+name, func(t string) bool { return strings.HasPrefix(strings.ToUpper(t), "V=DMARC1") })

	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		var orgID *string
		var wasVerified bool
		if err := tx.QueryRow(ctx, "select org_id::text, verified_at is not null from domains where name = $1 for update",
			name).Scan(&orgID, &wasVerified); err != nil {
			return err
		}
		allOK := mxOK && spfOK && dkimOK && dmarcOK
		if _, err := tx.Exec(ctx, `update domains set mx_ok = $2, spf_ok = $3, dkim_ok = $4, dmarc_ok = $5, last_checked_at = now(),
			verified_at = case when verified_at is null and $6 then now() else verified_at end where name = $1`,
			name, mxOK, spfOK, dkimOK, dmarcOK, allOK); err != nil {
			return err
		}
		if wasVerified || !allOK || orgID == nil {
			return nil
		}
		verified, err := s.scanDomain(tx.QueryRow(ctx, "select "+domainCols+" from domains where name = $1", name))
		if err != nil {
			return err
		}
		return insertEvent(ctx, tx, *orgID, "", "domain.verified", map[string]any{"domain": verified})
	})
}
