package core

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bertodrew/multi.ai.inbox/internal/auth"
)

// Roles a person can hold in an organization. Agents never hold one: they use
// API keys, whose scopes stay independent of people.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

func ValidRole(role string) bool {
	return role == RoleOwner || role == RoleAdmin || role == RoleMember
}

// PermissionsForRole maps a role onto the API key permissions of the same
// name, so one permission check serves both callers. owner adds the
// explicit-only ones: approving a held message and erasing the tenant are
// human decisions, and the owner is the human who answers for the tenant.
func PermissionsForRole(role string) []string {
	switch role {
	case RoleOwner:
		perms := append([]string{}, AllPermissions...)
		perms = append(perms, ExplicitPermissions...)
		return sortedUnique(perms)
	case RoleAdmin:
		perms := append([]string{}, AllPermissions...)
		perms = append(perms, PermMessageRelease)
		return sortedUnique(perms)
	default: // member, and anything unknown, gets the smallest useful set
		return sortedUnique([]string{PermInboxRead, PermMessageRead, PermMessageSend, PermMessageUpdate,
			PermDomainRead, PermWebhookRead, PermAPIKeyRead, PermBillingRead, PermTicketRead})
	}
}

func sortedUnique(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}

// AuthConfig holds the settings of the user authentication surface. A nil
// Auth, or a secret shorter than 32 bytes, disables every endpoint with 501
// instead of falling back to something weaker.
type AuthConfig struct {
	Secret     []byte
	AccessTTL  time.Duration // default 15 minutes
	RefreshTTL time.Duration // default 30 days
	MagicTTL   time.Duration // default 15 minutes
	StateTTL   time.Duration // default 10 minutes
	WebOrigin  string        // where /login lives, for browser redirects
	MailFrom   string
	Mailer     auth.Mailer // nil disables magic links
	Google     auth.Provider
	GitHub     auth.Provider
	// HTTPClient talks to the identity providers; nil uses http.DefaultClient.
	HTTPClient *http.Client
}

const jwtIssuer = "multi.ai.inbox"

func (s *Service) authConfig() (*AuthConfig, error) {
	if s.Auth == nil || len(s.Auth.Secret) < 32 {
		return nil, errf(http.StatusNotImplemented, "auth_disabled",
			"user authentication is not configured; set MAI_AUTH_SECRET to at least 32 bytes")
	}
	return s.Auth, nil
}

func (c *AuthConfig) accessTTL() time.Duration {
	if c.AccessTTL > 0 {
		return c.AccessTTL
	}
	return 15 * time.Minute
}

func (c *AuthConfig) refreshTTL() time.Duration {
	if c.RefreshTTL > 0 {
		return c.RefreshTTL
	}
	return 30 * 24 * time.Hour
}

func (c *AuthConfig) magicTTL() time.Duration {
	if c.MagicTTL > 0 {
		return c.MagicTTL
	}
	return 15 * time.Minute
}

func (c *AuthConfig) stateTTL() time.Duration {
	if c.StateTTL > 0 {
		return c.StateTTL
	}
	return 10 * time.Minute
}

// MagicTTLSeconds is the lifetime of a magic link, in seconds, for clients.
func (c *AuthConfig) MagicTTLSeconds() int { return int(c.magicTTL().Seconds()) }

func (c *AuthConfig) provider(name string) (auth.Provider, error) {
	switch name {
	case "google":
		return c.Google, nil
	case "github":
		return c.GitHub, nil
	default:
		return auth.Provider{}, Validation("unknown provider %q; valid: google, github", name)
	}
}

// User is a person. The password does not exist: the address is the identity,
// proved by a magic link or by the provider.
type User struct {
	ID          string     `json:"user_id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Provider    string     `json:"provider"`
	ProviderID  string     `json:"provider_id,omitempty"`
	AvatarURL   string     `json:"avatar_url,omitempty"`
	Role        string     `json:"role,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// SessionGrant is what a successful sign in returns: a short lived access
// token, the refresh token that replaces it, and the account it belongs to.
type SessionGrant struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	TokenType        string    `json:"token_type"`
	ExpiresIn        int       `json:"expires_in"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	User             *User     `json:"user"`
	Organization     *Org      `json:"organization"`
	Role             string    `json:"role"`
}

// OAuthStart is the answer to a start request: the browser goes to
// AuthorizeURL, which comes back to the callback with the same state.
type OAuthStart struct {
	Provider     string    `json:"provider"`
	AuthorizeURL string    `json:"authorize_url"`
	State        string    `json:"state"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func NormalizeEmail(raw string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(raw))
	if len(e) < 5 || len(e) > 254 {
		return "", Validation("email must be 5 to 254 characters")
	}
	addr, err := mail.ParseAddress(e)
	if err != nil || !strings.EqualFold(addr.Address, e) {
		return "", Validation("email is not a valid address")
	}
	local, domain, ok := strings.Cut(e, "@")
	if !ok || local == "" || !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", Validation("email is not a valid address")
	}
	return e, nil
}

// SafeRedirect accepts only a path on our own site, never another host: the
// login page hands redirect_after back to us and we must not become an open
// redirect.
func SafeRedirect(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.ContainsAny(p, "\\\r\n") {
		return ""
	}
	return p
}

// RequestMagicLink stores a single use token and emails it. The answer is the
// same for a known and an unknown address, so the endpoint never reveals who
// has an account.
func (s *Service) RequestMagicLink(ctx context.Context, email, linkBase, redirectAfter, ip string) error {
	cfg, err := s.authConfig()
	if err != nil {
		return err
	}
	if cfg.Mailer == nil {
		return errf(http.StatusNotImplemented, "auth_disabled", "magic links are not configured; set MAI_SMTP_RELAY")
	}
	addr, err := NormalizeEmail(email)
	if err != nil {
		return err
	}
	token, err := auth.NewToken(32)
	if err != nil {
		return err
	}
	now := s.now()
	err = s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		// One live link per address: asking again replaces the previous one.
		if _, err := tx.Exec(ctx, "update magic_link_tokens set used_at = now() where email = $1 and used_at is null", addr); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `insert into magic_link_tokens (token_hash, email, redirect_after, ip, expires_at)
			values ($1, $2, $3, $4, $5)`, auth.HashToken(token), addr, SafeRedirect(redirectAfter), ip, now.Add(cfg.magicTTL()))
		return err
	})
	if err != nil {
		return err
	}
	link := strings.TrimRight(linkBase, "/") + "/v0/auth/callback?token=" + url.QueryEscape(token)
	minutes := int(cfg.magicTTL().Minutes())
	text := "Sign in to multi.ai.inbox\n\n" +
		"Open this link to sign in. It works once and expires in " + strconv.Itoa(minutes) + " minutes.\n\n" +
		link + "\n\n" +
		"If you did not ask for this link, ignore this message: the link stays unusable until it expires.\n"
	if err := cfg.Mailer.Send(ctx, auth.Message{To: addr, Subject: "Your sign in link", Text: text}); err != nil {
		s.log().Error("magic link delivery failed", "err", err)
		return errf(http.StatusBadGateway, "email_delivery_failed", "the sign in email could not be sent, try again")
	}
	return nil
}

// CompleteMagicLink consumes the token and signs the person in. A token works
// once: the row is marked used in the same statement that reads it, so two
// clicks of the same link cannot both succeed.
func (s *Service) CompleteMagicLink(ctx context.Context, token, ip, userAgent string) (*SessionGrant, error) {
	cfg, err := s.authConfig()
	if err != nil {
		return nil, err
	}
	if !auth.WellFormedToken(token, 32) {
		return nil, errf(http.StatusBadRequest, "invalid_magic_link", "this sign in link is not valid")
	}
	var email string
	err = s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `update magic_link_tokens set used_at = now()
			where token_hash = $1 and used_at is null and expires_at > now()
			returning email`, auth.HashToken(token)).Scan(&email)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errf(http.StatusBadRequest, "invalid_magic_link", "this sign in link is invalid, already used, or expired")
	}
	if err != nil {
		return nil, err
	}
	identity := &auth.Identity{Provider: "magic_link", Email: email, EmailVerified: true}
	return s.signIn(ctx, cfg, identity, ip, userAgent)
}

// StartOAuth stores the PKCE verifier and the state, and returns the URL the
// browser must visit.
func (s *Service) StartOAuth(ctx context.Context, name, redirectURI, redirectAfter string) (*OAuthStart, error) {
	cfg, err := s.authConfig()
	if err != nil {
		return nil, err
	}
	p, err := cfg.provider(name)
	if err != nil {
		return nil, err
	}
	if !p.Configured() {
		return nil, errf(http.StatusNotImplemented, "provider_not_configured",
			"sign in with %s is not configured; set the client id and secret", name)
	}
	state, err := auth.NewToken(32)
	if err != nil {
		return nil, err
	}
	verifier, err := auth.NewVerifier()
	if err != nil {
		return nil, err
	}
	expires := s.now().Add(cfg.stateTTL())
	err = s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "delete from oauth_states where expires_at < now()"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `insert into oauth_states (state_hash, provider, code_verifier, redirect_uri, redirect_after, expires_at)
			values ($1, $2, $3, $4, $5, $6)`, auth.HashToken(state), p.Name, verifier, redirectURI, SafeRedirect(redirectAfter), expires)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &OAuthStart{Provider: p.Name, AuthorizeURL: p.AuthorizeURL(state, auth.ChallengeS256(verifier), redirectURI),
		State: state, ExpiresAt: expires}, nil
}

// CompleteOAuth redeems the state and the code, then signs the person in. The
// state row is deleted as it is read, so a replayed callback finds nothing.
// The provider comes from the stored state, never from the query string: a
// callback cannot claim to be a different provider than the one that started.
func (s *Service) CompleteOAuth(ctx context.Context, code, state, ip, userAgent string) (*SessionGrant, error) {
	cfg, err := s.authConfig()
	if err != nil {
		return nil, err
	}
	if code == "" || state == "" {
		return nil, Validation("code and state are required")
	}
	if !auth.WellFormedToken(state, 32) {
		return nil, errf(http.StatusBadRequest, "invalid_state", "this sign in attempt is invalid or expired")
	}
	var name, verifier, redirectURI string
	err = s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `delete from oauth_states where state_hash = $1 and expires_at > now()
			returning provider, code_verifier, redirect_uri`, auth.HashToken(state)).Scan(&name, &verifier, &redirectURI)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errf(http.StatusBadRequest, "invalid_state", "this sign in attempt is invalid or expired")
	}
	if err != nil {
		return nil, err
	}
	p, err := cfg.provider(name)
	if err != nil {
		return nil, err
	}
	identity, err := p.Code(ctx, cfg.HTTPClient, code, verifier, redirectURI)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrCodeRejected):
			return nil, errf(http.StatusUnauthorized, "code_rejected", "the provider did not accept this sign in attempt")
		case errors.Is(err, auth.ErrEmailUnverified):
			return nil, errf(http.StatusForbidden, "email_not_verified", "the provider did not confirm a verified email address")
		default:
			s.log().Error("oauth exchange failed", "provider", p.Name, "err", err)
			return nil, errf(http.StatusBadGateway, "provider_unavailable", "the identity provider did not answer, try again")
		}
	}
	if !identity.EmailVerified || identity.Email == "" {
		return nil, errf(http.StatusForbidden, "email_not_verified", "the provider did not confirm a verified email address")
	}
	email, err := NormalizeEmail(identity.Email)
	if err != nil {
		return nil, errf(http.StatusForbidden, "email_not_verified", "the provider returned an address this platform cannot accept")
	}
	identity.Email = email
	return s.signIn(ctx, cfg, identity, ip, userAgent)
}

// signIn finds or creates the account behind an identity, picks the
// organization it belongs to and issues a session.
func (s *Service) signIn(ctx context.Context, cfg *AuthConfig, identity *auth.Identity, ip, userAgent string) (*SessionGrant, error) {
	var grant *SessionGrant
	err := s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		user, org, role, err := s.accountFor(ctx, tx, identity)
		if err != nil {
			return err
		}
		grant, err = s.issueSession(ctx, tx, cfg, user, org, role, ip, userAgent)
		return err
	})
	if err != nil {
		return nil, err
	}
	return grant, nil
}

const userCols = "id, email, name, provider, coalesce(provider_id, ''), avatar_url, created_at, last_login_at"

func scanUser(row pgx.Row, u *User) error {
	return row.Scan(&u.ID, &u.Email, &u.Name, &u.Provider, &u.ProviderID, &u.AvatarURL, &u.CreatedAt, &u.LastLoginAt)
}

// accountFor resolves an identity to a user, an organization and a role,
// creating the user and a first organization the first time. It runs in the
// caller's transaction so a failure leaves nothing half created.
func (s *Service) accountFor(ctx context.Context, tx pgx.Tx, id *auth.Identity) (*User, *Org, string, error) {
	u := &User{}
	var err error
	if id.ProviderID != "" {
		err = scanUser(tx.QueryRow(ctx, "select "+userCols+" from users where provider = $1 and provider_id = $2", id.Provider, id.ProviderID), u)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, "", err
		}
	}
	if err != nil || u.ID == "" {
		// The address decides: an account made with a magic link and later
		// opened with GitHub stays one account.
		u = &User{}
		err = scanUser(tx.QueryRow(ctx, "select "+userCols+" from users where email = $1", id.Email), u)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, "", err
		}
	}
	switch {
	case err == nil && u.ID != "":
		var verified any
		if id.EmailVerified {
			verified = s.now()
		}
		name := id.Name
		avatar := id.AvatarURL
		if _, err := tx.Exec(ctx, `update users set last_login_at = now(), updated_at = now(),
				name = case when name = '' then $2 else name end,
				avatar_url = case when avatar_url = '' then $3 else avatar_url end,
				email_verified_at = coalesce(email_verified_at, $4)
			where id = $1`, u.ID, name, avatar, verified); err != nil {
			return nil, nil, "", err
		}
	case errors.Is(err, pgx.ErrNoRows):
		providerID := id.ProviderID
		var pid any
		if providerID != "" {
			pid = providerID
		}
		var verified any
		if id.EmailVerified {
			verified = s.now()
		}
		if err := scanUser(tx.QueryRow(ctx, `insert into users (email, provider, provider_id, name, avatar_url, email_verified_at, last_login_at)
				values ($1, $2, $3, $4, $5, $6, now()) returning `+userCols,
			id.Email, id.Provider, pid, id.Name, id.AvatarURL, verified), u); err != nil {
			return nil, nil, "", err
		}
	default:
		return nil, nil, "", err
	}

	org := &Org{}
	var role string
	err = scanOrg(tx.QueryRow(ctx, `select o.id, o.name, o.plan, o.screen_before_notify
		from memberships m join organizations o on o.id = m.org_id
		where m.user_id = $1 order by m.created_at limit 1`, u.ID), org)
	if err == nil {
		if err := tx.QueryRow(ctx, "select role from memberships where user_id = $1 and org_id = $2", u.ID, org.ID).Scan(&role); err != nil {
			return nil, nil, "", err
		}
		u.Role = role
		return u, org, role, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, "", err
	}
	// First sign in: the person gets an organization of their own, as owner.
	name := organizationNameFor(u)
	if err := scanOrg(tx.QueryRow(ctx, `insert into organizations (name) values ($1) returning `+orgCols, name), org); err != nil {
		return nil, nil, "", err
	}
	if _, err := tx.Exec(ctx, `insert into memberships (org_id, user_id, role) values ($1, $2, $3)`,
		org.ID, u.ID, RoleOwner); err != nil {
		return nil, nil, "", err
	}
	u.Role = RoleOwner
	return u, org, RoleOwner, nil
}

// organizationNameFor names the organization a first sign in creates. The
// address is predictable and unique per person; the console lets them rename
// it once colleagues join.
func organizationNameFor(u *User) string {
	if len(u.Email) <= 200 {
		return u.Email
	}
	return u.Email[:200]
}

// issueSession inserts one session and returns its tokens. The refresh token
// is stored as a hash; the access token is signed and never stored.
func (s *Service) issueSession(ctx context.Context, tx pgx.Tx, cfg *AuthConfig, u *User, org *Org, role, ip, userAgent string) (*SessionGrant, error) {
	refresh, err := auth.NewToken(32)
	if err != nil {
		return nil, err
	}
	now := s.now()
	refreshExpires := now.Add(cfg.refreshTTL())
	var sessionID string
	if err := tx.QueryRow(ctx, `insert into sessions (user_id, org_id, refresh_hash, user_agent, ip, expires_at)
			values ($1, $2, $3, $4, $5, $6) returning id`,
		u.ID, org.ID, auth.HashToken(refresh), truncate(userAgent, 300), ip, refreshExpires).Scan(&sessionID); err != nil {
		return nil, err
	}
	accessExpires := now.Add(cfg.accessTTL())
	token, err := auth.Sign(cfg.Secret, auth.Claims{Subject: u.ID, SessionID: sessionID, OrgID: org.ID, Role: role,
		Issuer: jwtIssuer, IssuedAt: now.Unix(), ExpiresAt: accessExpires.Unix()})
	if err != nil {
		return nil, err
	}
	return &SessionGrant{AccessToken: token, RefreshToken: refresh, TokenType: "Bearer",
		ExpiresIn: int(accessExpires.Sub(now).Seconds()), ExpiresAt: accessExpires, RefreshExpiresAt: refreshExpires,
		User: u, Organization: org, Role: role}, nil
}

// Refresh exchanges a refresh token for a new session and revokes the old one:
// a refresh token is single use, so a stolen one is worth no more than the
// next rotation.
func (s *Service) Refresh(ctx context.Context, refreshToken, ip, userAgent string) (*SessionGrant, error) {
	cfg, err := s.authConfig()
	if err != nil {
		return nil, err
	}
	invalid := errf(http.StatusUnauthorized, "invalid_refresh_token", "this session is invalid, already used, or expired")
	if !auth.WellFormedToken(refreshToken, 32) {
		return nil, invalid
	}
	var grant *SessionGrant
	err = s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		var sessionID, userID, orgID, role string
		err := tx.QueryRow(ctx, `select s.id, s.user_id, s.org_id, m.role
			from sessions s join memberships m on m.org_id = s.org_id and m.user_id = s.user_id
			where s.refresh_hash = $1 and s.revoked_at is null and s.expires_at > now()`, auth.HashToken(refreshToken)).
			Scan(&sessionID, &userID, &orgID, &role)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "update sessions set revoked_at = now() where id = $1", sessionID); err != nil {
			return err
		}
		u := &User{}
		if err := scanUser(tx.QueryRow(ctx, "select "+userCols+" from users where id = $1", userID), u); err != nil {
			return err
		}
		org := &Org{}
		if err := scanOrg(tx.QueryRow(ctx, "select "+orgCols+" from organizations where id = $1", orgID), org); err != nil {
			return err
		}
		u.Role = role
		grant, err = s.issueSession(ctx, tx, cfg, u, org, role, ip, userAgent)
		return err
	})
	if err != nil {
		return nil, err
	}
	return grant, nil
}

// Logout revokes the session behind a refresh token or an access token. It is
// idempotent and never says whether the token existed.
func (s *Service) Logout(ctx context.Context, refreshToken, accessToken string) error {
	cfg, err := s.authConfig()
	if err != nil {
		return err
	}
	if refreshToken == "" && accessToken == "" {
		return Validation("send refresh_token or an access token")
	}
	return s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		if refreshToken != "" && auth.WellFormedToken(refreshToken, 32) {
			if _, err := tx.Exec(ctx, "update sessions set revoked_at = now() where refresh_hash = $1 and revoked_at is null",
				auth.HashToken(refreshToken)); err != nil {
				return err
			}
		}
		if accessToken != "" {
			claims, err := auth.Verify(cfg.Secret, accessToken)
			if err == nil {
				if _, err := tx.Exec(ctx, "update sessions set revoked_at = now() where id = $1 and revoked_at is null", claims.SessionID); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// AuthenticateSession resolves an access token to a principal. The session row
// is checked on every request, so a logout ends access immediately instead of
// waiting for the token to expire.
func (s *Service) AuthenticateSession(ctx context.Context, token string) (*Principal, error) {
	cfg, err := s.authConfig()
	if err != nil {
		return nil, errf(http.StatusUnauthorized, "unknown_token", "invalid access token")
	}
	claims, err := auth.Verify(cfg.Secret, token)
	if err != nil {
		if errors.Is(err, auth.ErrTokenExpired) {
			return nil, errf(http.StatusUnauthorized, "token_expired", "the access token expired; refresh it")
		}
		return nil, errf(http.StatusUnauthorized, "unknown_token", "invalid access token")
	}
	if claims.Issuer != jwtIssuer {
		return nil, errf(http.StatusUnauthorized, "unknown_token", "invalid access token")
	}
	p := &Principal{Org: &Org{}, UserID: claims.Subject, SessionID: claims.SessionID}
	err = s.DB.WithSystem(ctx, func(tx pgx.Tx) error {
		var email, role string
		var sessionExpires time.Time
		err := tx.QueryRow(ctx, `select u.email, m.role, o.id, o.name, o.plan, o.screen_before_notify, s.expires_at
			from sessions s
			join users u on u.id = s.user_id
			join memberships m on m.org_id = s.org_id and m.user_id = s.user_id
			join organizations o on o.id = s.org_id
			where s.id = $1 and s.user_id = $2 and s.revoked_at is null and s.expires_at > now()`,
			claims.SessionID, claims.Subject).
			Scan(&email, &role, &p.Org.ID, &p.Org.Name, &p.Org.Plan, &p.Org.ScreenBeforeNotify, &sessionExpires)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // the session was revoked or expired: 401 below
		}
		if err != nil {
			return err
		}
		p.UserEmail = email
		p.Role = role
		p.Permissions = PermissionsForRole(role)
		expires := time.Unix(claims.ExpiresAt, 0)
		p.ExpiresAt = &expires
		return nil
	})
	if err != nil {
		return nil, err
	}
	if p.Org.ID == "" {
		return nil, errf(http.StatusUnauthorized, "session_revoked", "this session ended; sign in again")
	}
	return p, nil
}

func truncate(v string, n int) string {
	if len(v) <= n {
		return v
	}
	return v[:n]
}
