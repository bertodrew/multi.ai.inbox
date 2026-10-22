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
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/keys"
)

type Org struct {
	ID   string `json:"organization_id"`
	Name string `json:"name"`
	Plan string `json:"plan"`
}

func (s *Service) CreateOrg(ctx context.Context, name string) (*Org, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return nil, Validation("name must be 1 to 200 characters")
	}
	o := &Org{Name: name}
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "insert into organizations (name) values ($1) returning id, plan", name).Scan(&o.ID, &o.Plan)
	})
	return o, err
}

type CreatedKey struct {
	ID     string `json:"api_key_id"`
	Key    string `json:"api_key"`
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
}

func (s *Service) CreateKey(ctx context.Context, orgID, name string) (*CreatedKey, error) {
	plain, prefix, hash, err := keys.New()
	if err != nil {
		return nil, err
	}
	k := &CreatedKey{Key: plain, Prefix: prefix, Name: name}
	err = s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "insert into api_keys (org_id, name, prefix, hash) values ($1, $2, $3, $4) returning id",
			orgID, name, prefix, hash).Scan(&k.ID)
	})
	return k, err
}

// Authenticate resolves an API key to its organization.
func (s *Service) Authenticate(ctx context.Context, plain string) (*Org, error) {
	if !keys.WellFormed(plain) {
		return nil, errf(http.StatusUnauthorized, "unknown_api_key", "invalid API key")
	}
	o := &Org{}
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `update api_keys k set last_used_at = now()
			from organizations o
			where k.hash = $1 and k.revoked_at is null and o.id = k.org_id
			returning o.id, o.name, o.plan`, keys.Hash(plain)).Scan(&o.ID, &o.Name, &o.Plan)
		if errors.Is(err, pgx.ErrNoRows) {
			return errf(http.StatusUnauthorized, "unknown_api_key", "invalid API key")
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (s *Service) GetOrg(ctx context.Context, orgID string) (*Org, error) {
	o := &Org{}
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select id, name, plan from organizations where id = $1", orgID).Scan(&o.ID, &o.Name, &o.Plan)
	})
	return o, err
}

type DNSRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Domain struct {
	ID      string      `json:"domain_id"`
	Name    string      `json:"domain"`
	Shared  bool        `json:"shared"`
	Records []DNSRecord `json:"records"`
}

var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// AddDomain registers a sending and receiving domain with a fresh DKIM key.
// orgID empty means a shared platform domain. mxHost is the public MX name.
func (s *Service) AddDomain(ctx context.Context, orgID, name, mxHost string) (*Domain, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !domainRe.MatchString(name) {
		return nil, Validation("invalid domain %q", name)
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("dkim key: %w", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	pub, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, err
	}
	d := &Domain{Name: name, Shared: orgID == ""}
	var org any
	if orgID != "" {
		org = orgID
	}
	err = s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `insert into domains (org_id, name, dkim_private_key) values ($1, $2, $3) returning id`,
			org, name, string(privPEM)).Scan(&d.ID)
		if isUniqueViolation(err) {
			return errf(http.StatusConflict, "already_exists", "domain %s already exists", name)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	d.Records = []DNSRecord{
		{Type: "MX", Name: name, Value: "10 " + mxHost + "."},
		{Type: "TXT", Name: name, Value: "v=spf1 mx -all"},
		{Type: "TXT", Name: "mai1._domainkey." + name, Value: "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(pub)},
		{Type: "TXT", Name: "_dmarc." + name, Value: "v=DMARC1; p=quarantine; rua=mailto:dmarc@" + name},
	}
	return d, nil
}
