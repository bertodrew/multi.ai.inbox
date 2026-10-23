package aimulti

import (
	"context"
	"net/http"
)

// CreateInbox creates an inbox, or returns the existing one when the same
// client id was used before.
func (c *Client) CreateInbox(ctx context.Context, req CreateInboxRequest) (*Inbox, error) {
	out := &Inbox{}
	if err := c.sendJSON(ctx, http.MethodPost, "/inboxes", req, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListInboxes lists the organization's inboxes, newest first.
func (c *Client) ListInboxes(ctx context.Context, opts ListOptions) (*InboxList, error) {
	out := &InboxList{}
	if err := c.getJSON(ctx, "/inboxes", opts.values(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetInbox reads one inbox by address.
func (c *Client) GetInbox(ctx context.Context, address string) (*Inbox, error) {
	out := &Inbox{}
	if err := c.getJSON(ctx, "/inboxes/"+escape(address), nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateInbox renames an inbox or turns it into a support intake.
func (c *Client) UpdateInbox(ctx context.Context, address string, req UpdateInboxRequest) (*Inbox, error) {
	out := &Inbox{}
	if err := c.sendJSON(ctx, http.MethodPatch, "/inboxes/"+escape(address), req, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteInbox deletes an inbox and its mail. The address is quarantined for 90
// days, so no organization can claim it in that window.
func (c *Client) DeleteInbox(ctx context.Context, address string) error {
	return c.sendJSON(ctx, http.MethodDelete, "/inboxes/"+escape(address), nil, nil, nil)
}

// ListThreads lists threads of one inbox. Pass an empty address to list the
// organization's threads instead.
func (c *Client) ListThreads(ctx context.Context, address string, opts ListOptions) (*ThreadList, error) {
	path := "/threads"
	if address != "" {
		path = "/inboxes/" + escape(address) + "/threads"
	}
	out := &ThreadList{}
	if err := c.getJSON(ctx, path, opts.values(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetThread reads a thread with the messages the caller may see.
func (c *Client) GetThread(ctx context.Context, threadID string, includeReview bool) (*Thread, error) {
	opts := ListOptions{IncludeReview: includeReview}
	out := &Thread{}
	if err := c.getJSON(ctx, "/threads/"+escape(threadID), opts.values(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetInboxThread reads a thread and checks that it belongs to the inbox.
func (c *Client) GetInboxThread(ctx context.Context, address, threadID string, includeReview bool) (*Thread, error) {
	opts := ListOptions{IncludeReview: includeReview}
	path := "/inboxes/" + escape(address) + "/threads/" + escape(threadID)
	out := &Thread{}
	if err := c.getJSON(ctx, path, opts.values(), out); err != nil {
		return nil, err
	}
	return out, nil
}
