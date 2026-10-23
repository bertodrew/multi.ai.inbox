package aimulti

import (
	"context"
	"net/http"
)

// CreateDomain adds a custom domain and returns the DNS records to publish. A
// fresh DKIM key is generated for each domain.
func (c *Client) CreateDomain(ctx context.Context, domain string) (*Domain, error) {
	body := struct {
		Domain string `json:"domain"`
	}{domain}
	out := &Domain{}
	if err := c.sendJSON(ctx, http.MethodPost, "/domains", body, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListDomains lists the organization's own domains and the shared domain.
func (c *Client) ListDomains(ctx context.Context) ([]Domain, error) {
	out := &DomainList{}
	if err := c.getJSON(ctx, "/domains", nil, out); err != nil {
		return nil, err
	}
	return out.Domains, nil
}

// GetDomain reads a domain with the status of each DNS record.
func (c *Client) GetDomain(ctx context.Context, domain string) (*Domain, error) {
	out := &Domain{}
	if err := c.getJSON(ctx, "/domains/"+escape(domain), nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteDomain deletes a domain that has no inboxes.
func (c *Client) DeleteDomain(ctx context.Context, domain string) error {
	return c.sendJSON(ctx, http.MethodDelete, "/domains/"+escape(domain), nil, nil, nil)
}

// VerifyDomain checks the DNS records now. The scheduler also checks every five
// minutes for seven days after the domain is added.
func (c *Client) VerifyDomain(ctx context.Context, domain string) (*Domain, error) {
	out := &Domain{}
	if err := c.sendJSON(ctx, http.MethodPost, "/domains/"+escape(domain)+"/verify", nil, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateWebhook registers an HTTPS endpoint for events. Secret is set in the
// response and returned only once.
func (c *Client) CreateWebhook(ctx context.Context, req CreateWebhookRequest) (*Webhook, error) {
	out := &Webhook{}
	if err := c.sendJSON(ctx, http.MethodPost, "/webhooks", req, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListWebhooks lists the organization's webhooks.
func (c *Client) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	out := &WebhookList{}
	if err := c.getJSON(ctx, "/webhooks", nil, out); err != nil {
		return nil, err
	}
	return out.Webhooks, nil
}

// DeleteWebhook deletes a webhook.
func (c *Client) DeleteWebhook(ctx context.Context, webhookID string) error {
	return c.sendJSON(ctx, http.MethodDelete, "/webhooks/"+escape(webhookID), nil, nil, nil)
}

// CreateAPIKey issues a key with at most the caller's permissions. An empty
// Permissions copies them. Key is set in the response and returned only once.
func (c *Client) CreateAPIKey(ctx context.Context, req CreateAPIKeyRequest) (*APIKey, error) {
	out := &APIKey{}
	if err := c.sendJSON(ctx, http.MethodPost, "/api-keys", req, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListAPIKeys lists the active keys of the organization.
func (c *Client) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	out := &APIKeyList{}
	if err := c.getJSON(ctx, "/api-keys", nil, out); err != nil {
		return nil, err
	}
	return out.APIKeys, nil
}

// RevokeAPIKey revokes a key immediately.
func (c *Client) RevokeAPIKey(ctx context.Context, keyID string) error {
	return c.sendJSON(ctx, http.MethodDelete, "/api-keys/"+escape(keyID), nil, nil, nil)
}

// ListAuditLogs reads the organization's security audit log, newest first.
// Limit may be zero for the API default of 50.
func (c *Client) ListAuditLogs(ctx context.Context, limit int) ([]AuditEntry, error) {
	q := ListOptions{Limit: limit}.values()
	out := &AuditList{}
	if err := c.getJSON(ctx, "/audit-logs", q, out); err != nil {
		return nil, err
	}
	return out.Entries, nil
}

// Billing reads the plan and the Stripe subscription state.
func (c *Client) Billing(ctx context.Context) (*Billing, error) {
	out := &Billing{}
	if err := c.getJSON(ctx, "/billing", nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateCheckout starts a Stripe Checkout for a paid plan and returns the URL to
// send the customer to. The plan changes when Stripe confirms the payment.
func (c *Client) CreateCheckout(ctx context.Context, req CheckoutRequest) (*BillingSession, error) {
	out := &BillingSession{}
	if err := c.sendJSON(ctx, http.MethodPost, "/billing/checkout", req, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreatePortal opens the Stripe customer portal: plan changes, payment method,
// invoices and cancellation.
func (c *Client) CreatePortal(ctx context.Context, returnURL string) (*BillingSession, error) {
	body := struct {
		ReturnURL string `json:"return_url,omitempty"`
	}{returnURL}
	out := &BillingSession{}
	if err := c.sendJSON(ctx, http.MethodPost, "/billing/portal", body, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Export returns the organization's own data as one document: settings,
// inboxes, domains, webhooks, key metadata, counts and the audit log. Secrets
// are never included.
func (c *Client) Export(ctx context.Context) (*OrgExport, error) {
	out := &OrgExport{}
	if err := c.getJSON(ctx, "/export", nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteOrganization deletes the organization and everything it owns. Confirm
// must equal the organization name exactly. The answer carries the export taken
// before deletion.
func (c *Client) DeleteOrganization(ctx context.Context, confirm string) (*DeleteOrganizationResult, error) {
	body := struct {
		Confirm string `json:"confirm"`
	}{confirm}
	out := &DeleteOrganizationResult{}
	if err := c.sendJSON(ctx, http.MethodDelete, "/organization", body, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}
