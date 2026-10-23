package aimulti

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

// The types below mirror components/schemas in openapi/openapi.yaml. Field
// names and shapes are the wire format: pass these values straight to a
// method and read the response without translating anything.

// Me is the authenticated caller, an API key or a person's session.
type Me struct {
	Auth               string    `json:"auth"` // api_key | session
	OrganizationID     string    `json:"organization_id"`
	Name               string    `json:"name"`
	Plan               string    `json:"plan"` // free | builder | growth | team
	ScreenBeforeNotify bool      `json:"screen_before_notify"`
	Role               string    `json:"role,omitempty"` // only for a session
	APIKeyID           string    `json:"api_key_id,omitempty"`
	User               *MeUser   `json:"user,omitempty"`
	Organization       *Org      `json:"organization,omitempty"`
	Permissions        []string  `json:"permissions"`
	ExpiresAt          time.Time `json:"expires_at"`
}

// MeUser is the person behind a session.
type MeUser struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

// Org is an organization and its plan level settings.
type Org struct {
	OrganizationID     string `json:"organization_id"`
	Name               string `json:"name"`
	Plan               string `json:"plan"`
	ScreenBeforeNotify bool   `json:"screen_before_notify"`
}

// User is an account.
type User struct {
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	Provider    string    `json:"provider"` // magic_link | google | github
	Role        string    `json:"role"`     // owner | admin | member
	CreatedAt   time.Time `json:"created_at"`
	LastLoginAt time.Time `json:"last_login_at"`
}

// Session is a signed in person: a short lived access token and a single use
// refresh token.
type Session struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	TokenType        string    `json:"token_type"`
	ExpiresIn        int       `json:"expires_in"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	Role             string    `json:"role"`
	User             *User     `json:"user"`
	Organization     *Org      `json:"organization"`
}

// OAuthStart is where to send a browser to sign in with Google or GitHub.
type OAuthStart struct {
	Provider     string    `json:"provider"`
	AuthorizeURL string    `json:"authorize_url"`
	State        string    `json:"state"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// CreateInboxRequest asks for an inbox. Repeating the same ClientID returns
// the same inbox instead of creating a second one.
type CreateInboxRequest struct {
	Username    string `json:"username,omitempty"`
	Domain      string `json:"domain,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	ClientID    string `json:"client_id,omitempty"`
}

// UpdateInboxRequest changes an inbox; nil fields are left alone.
type UpdateInboxRequest struct {
	DisplayName  *string `json:"display_name,omitempty"`
	TicketIntake *bool   `json:"ticket_intake,omitempty"`
}

// Inbox is an address that receives and sends mail.
type Inbox struct {
	ID                  string     `json:"id"`
	InboxID             string     `json:"inbox_id"` // the address
	Email               string     `json:"email"`
	DisplayName         string     `json:"display_name"`
	ClientID            string     `json:"client_id,omitempty"`
	TicketIntake        bool       `json:"ticket_intake"`
	SendingPausedAt     *time.Time `json:"sending_paused_at,omitempty"`
	SendingPausedReason string     `json:"sending_paused_reason,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// InboxList is one page of inboxes.
type InboxList struct {
	Count         int     `json:"count"`
	NextPageToken string  `json:"next_page_token,omitempty"`
	Inboxes       []Inbox `json:"inboxes"`
}

// Thread groups the messages of one conversation.
type Thread struct {
	ThreadID     string    `json:"thread_id"`
	InboxID      string    `json:"inbox_id"`
	Subject      string    `json:"subject"`
	Preview      string    `json:"preview"`
	Labels       []string  `json:"labels"`
	MessageCount int       `json:"message_count"`
	Timestamp    time.Time `json:"timestamp"`
	Messages     []Message `json:"messages,omitempty"`
}

// ThreadList is one page of threads.
type ThreadList struct {
	Count         int      `json:"count"`
	NextPageToken string   `json:"next_page_token,omitempty"`
	Threads       []Thread `json:"threads"`
}

// Message is a mail message. Text and HTML are empty in list responses.
type Message struct {
	ID                    string                 `json:"id"`
	MessageID             string                 `json:"message_id"`
	ThreadID              string                 `json:"thread_id"`
	InboxID               string                 `json:"inbox_id"`
	Direction             string                 `json:"direction"` // in | out
	From                  string                 `json:"from"`
	To                    []string               `json:"to"`
	CC                    []string               `json:"cc,omitempty"`
	Subject               string                 `json:"subject"`
	Text                  string                 `json:"text,omitempty"`
	HTML                  string                 `json:"html,omitempty"`
	ExtractedText         string                 `json:"extracted_text,omitempty"`
	Preview               string                 `json:"preview,omitempty"`
	Labels                []string               `json:"labels,omitempty"`
	InReplyTo             string                 `json:"in_reply_to,omitempty"`
	References            []string               `json:"references,omitempty"`
	Status                string                 `json:"status,omitempty"`
	AuthenticationResults *AuthenticationResults `json:"authentication_results,omitempty"`
	Gatekeeper            *GatekeeperVerdict     `json:"gatekeeper,omitempty"`
	Screening             string                 `json:"screening,omitempty"` // review | allowed | released
	Attachments           []AttachmentMeta       `json:"attachments,omitempty"`
	CreatedAt             time.Time              `json:"created_at"`
}

// AuthenticationResults is what the SMTP edge verified for inbound mail.
type AuthenticationResults struct {
	DKIM        string  `json:"dkim,omitempty"`
	SPF         string  `json:"spf,omitempty"`
	DMARC       string  `json:"dmarc,omitempty"`
	DMARCPolicy string  `json:"dmarc_policy,omitempty"`
	Antivirus   string  `json:"antivirus,omitempty"`
	SpamScore   float64 `json:"spam_score,omitempty"`
	SpamAction  string  `json:"spam_action,omitempty"`
	Internal    bool    `json:"internal,omitempty"`
}

// GatekeeperVerdict is the security screening result.
type GatekeeperVerdict struct {
	Status  string   `json:"status,omitempty"`  // pending | complete | unavailable | timeout
	Verdict string   `json:"verdict,omitempty"` // clear | review
	Score   float64  `json:"score,omitempty"`
	Reasons []string `json:"reasons,omitempty"`
	Model   string   `json:"model,omitempty"`
}

// AttachmentMeta describes an attachment without its bytes.
type AttachmentMeta struct {
	AttachmentID string `json:"attachment_id"`
	Filename     string `json:"filename"`
	ContentType  string `json:"content_type"`
	Size         int64  `json:"size"`
}

// MessageList is one page of messages.
type MessageList struct {
	Count         int       `json:"count"`
	NextPageToken string    `json:"next_page_token,omitempty"`
	Messages      []Message `json:"messages"`
}

// Attachment is an attachment to send. Data is base64 encoded on the wire.
type Attachment struct {
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Data        []byte `json:"-"`
}

type attachmentWire struct {
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Content     string `json:"content"`
}

// MarshalJSON encodes Data as the base64 string the API expects.
func (a Attachment) MarshalJSON() ([]byte, error) {
	return json.Marshal(attachmentWire{
		Filename:    a.Filename,
		ContentType: a.ContentType,
		Content:     base64.StdEncoding.EncodeToString(a.Data),
	})
}

// SendRequest starts a new thread.
type SendRequest struct {
	To          []string     `json:"to"`
	CC          []string     `json:"cc,omitempty"`
	BCC         []string     `json:"bcc,omitempty"`
	Subject     string       `json:"subject,omitempty"`
	Text        string       `json:"text,omitempty"`
	HTML        string       `json:"html,omitempty"`
	Labels      []string     `json:"labels,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// ReplyRequest answers a message in its own thread.
type ReplyRequest struct {
	Text        string       `json:"text,omitempty"`
	HTML        string       `json:"html,omitempty"`
	ReplyAll    bool         `json:"reply_all,omitempty"`
	Labels      []string     `json:"labels,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// SendResult is a queued message.
type SendResult struct {
	MessageID string `json:"message_id"`
	ThreadID  string `json:"thread_id"`
}

// CreateDraftRequest saves a draft, optionally a reply, optionally scheduled.
type CreateDraftRequest struct {
	To          []string     `json:"to,omitempty"`
	CC          []string     `json:"cc,omitempty"`
	BCC         []string     `json:"bcc,omitempty"`
	Subject     string       `json:"subject,omitempty"`
	Text        string       `json:"text,omitempty"`
	HTML        string       `json:"html,omitempty"`
	Labels      []string     `json:"labels,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	InReplyTo   string       `json:"in_reply_to,omitempty"`
	ReplyAll    bool         `json:"reply_all,omitempty"`
	SendAt      *time.Time   `json:"send_at,omitempty"`
}

// UpdateDraftRequest replaces the fields that are set. CancelSendAt sends
// send_at: null, which unschedules a scheduled draft.
type UpdateDraftRequest struct {
	To           []string     `json:"to,omitempty"`
	CC           []string     `json:"cc,omitempty"`
	BCC          []string     `json:"bcc,omitempty"`
	Subject      *string      `json:"subject,omitempty"`
	Text         *string      `json:"text,omitempty"`
	HTML         *string      `json:"html,omitempty"`
	Labels       []string     `json:"labels,omitempty"`
	Attachments  []Attachment `json:"attachments,omitempty"`
	SendAt       *time.Time   `json:"send_at,omitempty"`
	CancelSendAt bool         `json:"-"`
}

// MarshalJSON adds send_at: null when CancelSendAt is set, since omitempty
// cannot express an explicit null.
func (u UpdateDraftRequest) MarshalJSON() ([]byte, error) {
	if !u.CancelSendAt {
		type plain UpdateDraftRequest
		return json.Marshal(plain(u))
	}
	return json.Marshal(struct {
		To          []string     `json:"to,omitempty"`
		CC          []string     `json:"cc,omitempty"`
		BCC         []string     `json:"bcc,omitempty"`
		Subject     *string      `json:"subject,omitempty"`
		Text        *string      `json:"text,omitempty"`
		HTML        *string      `json:"html,omitempty"`
		Labels      []string     `json:"labels,omitempty"`
		Attachments []Attachment `json:"attachments,omitempty"`
		SendAt      *time.Time   `json:"send_at"`
	}{u.To, u.CC, u.BCC, u.Subject, u.Text, u.HTML, u.Labels, u.Attachments, nil})
}

// Draft is a saved message.
type Draft struct {
	DraftID     string           `json:"draft_id"`
	InboxID     string           `json:"inbox_id"`
	ThreadID    string           `json:"thread_id,omitempty"`
	InReplyTo   string           `json:"in_reply_to,omitempty"`
	References  []string         `json:"references,omitempty"`
	To          []string         `json:"to,omitempty"`
	CC          []string         `json:"cc,omitempty"`
	BCC         []string         `json:"bcc,omitempty"`
	Subject     string           `json:"subject,omitempty"`
	Text        string           `json:"text,omitempty"`
	HTML        string           `json:"html,omitempty"`
	Labels      []string         `json:"labels,omitempty"`
	Attachments []AttachmentMeta `json:"attachments,omitempty"`
	SendAt      *time.Time       `json:"send_at,omitempty"`
	Status      string           `json:"status,omitempty"` // draft | scheduled | failed
	LastError   string           `json:"last_error,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// DraftList is one page of drafts.
type DraftList struct {
	Count         int     `json:"count"`
	NextPageToken string  `json:"next_page_token,omitempty"`
	Drafts        []Draft `json:"drafts"`
}

// DNSRecord is one record to publish for a custom domain.
type DNSRecord struct {
	Type   string `json:"type"` // MX | TXT
	Name   string `json:"name"`
	Value  string `json:"value"`
	Status string `json:"status"` // verified | missing
}

// Domain is a custom sending and receiving domain.
type Domain struct {
	DomainID      string      `json:"domain_id"`
	Domain        string      `json:"domain"`
	Shared        bool        `json:"shared"`
	Status        string      `json:"status"` // pending | verified
	Records       []DNSRecord `json:"records"`
	LastCheckedAt *time.Time  `json:"last_checked_at,omitempty"`
	VerifiedAt    *time.Time  `json:"verified_at,omitempty"`
}

// DomainList is the domain listing.
type DomainList struct {
	Count   int      `json:"count"`
	Domains []Domain `json:"domains"`
}

// Webhook delivers events to an HTTPS endpoint.
type Webhook struct {
	WebhookID  string    `json:"webhook_id"`
	URL        string    `json:"url"`
	EventTypes []string  `json:"event_types"`
	InboxIDs   []string  `json:"inbox_ids,omitempty"`
	Enabled    bool      `json:"enabled"`
	Secret     string    `json:"secret,omitempty"` // only on create
	CreatedAt  time.Time `json:"created_at"`
}

// CreateWebhookRequest registers an endpoint.
type CreateWebhookRequest struct {
	URL        string   `json:"url"`
	EventTypes []string `json:"event_types"`
	InboxIDs   []string `json:"inbox_ids,omitempty"`
}

// WebhookList is the webhook listing.
type WebhookList struct {
	Count    int       `json:"count"`
	Webhooks []Webhook `json:"webhooks"`
}

// APIKey is a scoped key. Key is set only in the create response.
type APIKey struct {
	ID          string     `json:"api_key_id"`
	Key         string     `json:"api_key,omitempty"`
	Prefix      string     `json:"prefix"`
	Name        string     `json:"name"`
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// CreateAPIKeyRequest issues a key with at most the caller's permissions. An
// empty Permissions copies them.
type CreateAPIKeyRequest struct {
	Name        string     `json:"name,omitempty"`
	Permissions []string   `json:"permissions,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// APIKeyList is the key listing.
type APIKeyList struct {
	Count   int      `json:"count"`
	APIKeys []APIKey `json:"api_keys"`
}

// AuditEntry is one security relevant action.
type AuditEntry struct {
	ID        int64           `json:"id"`
	Actor     string          `json:"actor"`
	Action    string          `json:"action"`
	Target    string          `json:"target"`
	IP        string          `json:"ip"`
	RequestID string          `json:"request_id"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// AuditList is the audit log listing.
type AuditList struct {
	Count   int          `json:"count"`
	Entries []AuditEntry `json:"entries"`
}

// Billing is the plan and subscription state.
type Billing struct {
	Plan              string     `json:"plan"`
	Status            string     `json:"status"`
	Interval          string     `json:"interval,omitempty"`
	CurrentPeriodEnd  *time.Time `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd bool       `json:"cancel_at_period_end"`
	PastDueSince      *time.Time `json:"past_due_since,omitempty"`
	GracePeriodEndsAt *time.Time `json:"grace_period_ends_at,omitempty"`
}

// CheckoutRequest starts a Stripe Checkout for a paid plan.
type CheckoutRequest struct {
	Plan       string `json:"plan"` // builder | growth | team
	Interval   string `json:"interval,omitempty"`
	SuccessURL string `json:"success_url,omitempty"`
	CancelURL  string `json:"cancel_url,omitempty"`
}

// BillingSession is a Stripe Checkout or portal URL.
type BillingSession struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
}

// ExportInbox is one inbox in a data export.
type ExportInbox struct {
	Address     string    `json:"address"`
	DisplayName string    `json:"display_name"`
	ClientID    string    `json:"client_id"`
	CreatedAt   time.Time `json:"created_at"`
	Messages    int       `json:"messages"`
}

// ExportDomain is one domain in a data export.
type ExportDomain struct {
	Domain     string    `json:"domain"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	VerifiedAt time.Time `json:"verified_at"`
}

// ExportWebhook is one webhook in a data export.
type ExportWebhook struct {
	WebhookID  string    `json:"webhook_id"`
	URL        string    `json:"url"`
	EventTypes []string  `json:"event_types"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
}

// ExportAPIKey is key metadata, never key material.
type ExportAPIKey struct {
	APIKeyID    string     `json:"api_key_id"`
	Name        string     `json:"name"`
	Permissions []string   `json:"permissions"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

// OrgExport is the organization's own data (GDPR Art. 15 and 20). Secrets are
// never included. Ticket records are kept as raw JSON because their shape
// grows with the support queue.
type OrgExport struct {
	Format       string          `json:"format"`
	GeneratedAt  time.Time       `json:"generated_at"`
	Organization *Org            `json:"organization"`
	Inboxes      []ExportInbox   `json:"inboxes"`
	Domains      []ExportDomain  `json:"domains"`
	Webhooks     []ExportWebhook `json:"webhooks"`
	APIKeys      []ExportAPIKey  `json:"api_keys"`
	Counts       map[string]int  `json:"counts"`
	AuditLog     []AuditEntry    `json:"audit_log"`
	Notice       string          `json:"notice"`
	Tickets      json.RawMessage `json:"tickets,omitempty"`
	TicketNotes  json.RawMessage `json:"ticket_notes,omitempty"`
	TicketEvents json.RawMessage `json:"ticket_events,omitempty"`
}

// DeletionResult reports what deleting an organization did.
type DeletionResult struct {
	OrganizationID       string   `json:"organization_id"`
	Deleted              bool     `json:"deleted"`
	BlobsQueued          int      `json:"blobs_queued"`
	AddressesQuarantined int      `json:"addresses_quarantined"`
	DomainsReleased      []string `json:"custom_domains_released"`
}

// DeletionSummary is the account of what was deleted.
type DeletionSummary struct {
	Organization   *Org           `json:"organization"`
	CreatedAt      time.Time      `json:"created_at"`
	ExportedAt     time.Time      `json:"exported_at"`
	Inboxes        []string       `json:"inboxes"`
	CustomDomains  []string       `json:"custom_domains"`
	Counts         map[string]int `json:"counts"`
	APIKeysRevoked int            `json:"api_keys_revoked"`
	AuditLog       []AuditEntry   `json:"audit_log"`
}

// DeleteOrganizationResult is the answer to DELETE /v1/organization: what was
// deleted and the export taken before deletion.
type DeleteOrganizationResult struct {
	Deletion DeletionResult   `json:"deletion"`
	Export   *DeletionSummary `json:"export_before_deletion"`
}

// TicketState and TicketPriority are the queue enums.
const (
	StateNew      = "new"
	StateOpen     = "open"
	StatePending  = "pending"
	StateResolved = "resolved"
	StateClosed   = "closed"

	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

// Ticket is a thread with state: the support queue.
type Ticket struct {
	ID            string     `json:"id"`
	Number        int        `json:"number"`
	InboxID       string     `json:"inbox_id"`
	ThreadID      string     `json:"thread_id"`
	Subject       string     `json:"subject"`
	Requester     string     `json:"requester"`
	State         string     `json:"state"`
	Priority      string     `json:"priority"`
	AssigneeUser  string     `json:"assignee_user,omitempty"`
	AssigneeKey   string     `json:"assignee_key,omitempty"`
	SLAPolicy     string     `json:"sla_policy,omitempty"`
	DueFirstReply *time.Time `json:"due_first_reply,omitempty"`
	DueResolution *time.Time `json:"due_resolution,omitempty"`
	FirstReplyAt  *time.Time `json:"first_reply_at,omitempty"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	ClosedAt      *time.Time `json:"closed_at,omitempty"`
	Tags          []string   `json:"tags,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TicketList is one page of tickets.
type TicketList struct {
	Count         int      `json:"count"`
	NextPageToken string   `json:"next_page_token,omitempty"`
	Tickets       []Ticket `json:"tickets"`
}

// TicketEvent is one entry in a ticket timeline.
type TicketEvent struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	Actor     string          `json:"actor"`
	Detail    json.RawMessage `json:"detail,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// TicketNote is an internal note. Notes have no path to the sender.
type TicketNote struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// TicketDetail is one ticket with its timeline and notes.
type TicketDetail struct {
	Ticket *Ticket       `json:"ticket"`
	Events []TicketEvent `json:"events"`
	Notes  []TicketNote  `json:"notes"`
}

// TicketStats is the queue counters.
type TicketStats struct {
	ByState                map[string]int `json:"by_state"`
	Open                   int            `json:"open"`
	Breaching              int            `json:"breaching"`
	Unassigned             int            `json:"unassigned"`
	MedianFirstReplySecond int            `json:"median_first_reply_seconds"`
}

// CreateTicketRequest opens a ticket on an existing thread.
type CreateTicketRequest struct {
	ThreadID     string   `json:"thread_id"`
	Priority     string   `json:"priority,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	AssigneeUser string   `json:"assignee_user,omitempty"`
	AssigneeKey  string   `json:"assignee_key,omitempty"`
}

// UpdateTicketRequest changes state, priority, tags or assignment. Nil fields
// are left alone.
type UpdateTicketRequest struct {
	State        *string  `json:"state,omitempty"`
	Priority     *string  `json:"priority,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	AssigneeUser *string  `json:"assignee_user,omitempty"`
	AssigneeKey  *string  `json:"assignee_key,omitempty"`
}

// EventEnvelope is the data of one streamed event: the same payload a webhook
// delivers.
type EventEnvelope struct {
	Type      string          `json:"type"`
	EventType string          `json:"event_type"`
	EventID   string          `json:"event_id"`
	CreatedAt time.Time       `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}
