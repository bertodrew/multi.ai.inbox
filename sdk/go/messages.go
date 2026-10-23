package aimulti

import (
	"context"
	"mime"
	"net/http"
	"net/url"
)

// File is downloaded bytes with the metadata the API reported.
type File struct {
	Filename    string
	ContentType string
	Data        []byte
}

// ListMessages lists message metadata of an inbox. Bodies are left out.
func (c *Client) ListMessages(ctx context.Context, address string, opts ListOptions) (*MessageList, error) {
	out := &MessageList{}
	path := "/inboxes/" + escape(address) + "/messages"
	if err := c.getJSON(ctx, path, opts.values(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetMessage reads one message with bodies, extracted text, authentication
// results and the gatekeeper verdict. A message may be named by uuid or by its
// RFC Message-ID.
func (c *Client) GetMessage(ctx context.Context, address, message string, includeReview bool) (*Message, error) {
	opts := ListOptions{IncludeReview: includeReview}
	out := &Message{}
	path := "/inboxes/" + escape(address) + "/messages/" + escape(message)
	if err := c.getJSON(ctx, path, opts.values(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateMessageLabels adds and removes custom labels.
func (c *Client) UpdateMessageLabels(ctx context.Context, address, message string, add, remove []string) (*Message, error) {
	body := struct {
		AddLabels    []string `json:"add_labels,omitempty"`
		RemoveLabels []string `json:"remove_labels,omitempty"`
	}{add, remove}
	out := &Message{}
	path := "/inboxes/" + escape(address) + "/messages/" + escape(message)
	if err := c.sendJSON(ctx, http.MethodPatch, path, body, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// RawMessage returns the RFC 822 source of a message.
func (c *Client) RawMessage(ctx context.Context, address, message string, includeReview bool) ([]byte, error) {
	q := ListOptions{IncludeReview: includeReview}.values()
	path := "/inboxes/" + escape(address) + "/messages/" + escape(message) + "/raw"
	raw, _, err := c.call(ctx, request{method: http.MethodGet, path: path, query: q, accept: "message/rfc822"})
	return raw, err
}

// GetAttachment downloads one attachment.
func (c *Client) GetAttachment(ctx context.Context, address, message, attachmentID string, includeReview bool) (*File, error) {
	q := ListOptions{IncludeReview: includeReview}.values()
	path := "/inboxes/" + escape(address) + "/messages/" + escape(message) + "/attachments/" + escape(attachmentID)
	raw, header, err := c.call(ctx, request{method: http.MethodGet, path: path, query: q})
	if err != nil {
		return nil, err
	}
	return &File{
		Filename:    filename(header.Get("Content-Disposition")),
		ContentType: header.Get("Content-Type"),
		Data:        raw,
	}, nil
}

// filename reads the filename parameter of a Content-Disposition header.
func filename(disposition string) string {
	if disposition == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return ""
	}
	return params["filename"]
}

// Send queues a new message in a new thread. WithIdempotencyKey makes a retry
// safe.
func (c *Client) Send(ctx context.Context, address string, req SendRequest, opts ...CallOption) (*SendResult, error) {
	out := &SendResult{}
	path := "/inboxes/" + escape(address) + "/messages/send"
	if err := c.sendJSON(ctx, http.MethodPost, path, req, opts, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Reply queues a reply in the thread of the message. WithIdempotencyKey makes a
// retry safe.
func (c *Client) Reply(ctx context.Context, address, message string, req ReplyRequest, opts ...CallOption) (*SendResult, error) {
	out := &SendResult{}
	path := "/inboxes/" + escape(address) + "/messages/" + escape(message) + "/reply"
	if err := c.sendJSON(ctx, http.MethodPost, path, req, opts, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ReleaseMessage makes a message held for review visible and fires
// message.received. The key needs message_release; the action is audited.
func (c *Client) ReleaseMessage(ctx context.Context, address, message string) (*Message, error) {
	out := &Message{}
	path := "/inboxes/" + escape(address) + "/messages/" + escape(message) + "/release"
	if err := c.sendJSON(ctx, http.MethodPost, path, nil, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateDraft saves a draft, optionally a reply, optionally scheduled with
// SendAt.
func (c *Client) CreateDraft(ctx context.Context, address string, req CreateDraftRequest) (*Draft, error) {
	out := &Draft{}
	path := "/inboxes/" + escape(address) + "/drafts"
	if err := c.sendJSON(ctx, http.MethodPost, path, req, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListDrafts lists drafts of an inbox, newest first.
func (c *Client) ListDrafts(ctx context.Context, address string, opts ListOptions) (*DraftList, error) {
	out := &DraftList{}
	path := "/inboxes/" + escape(address) + "/drafts"
	q := url.Values{}
	if opts.Limit > 0 {
		q.Set("limit", itoa(opts.Limit))
	}
	if opts.PageToken != "" {
		q.Set("page_token", opts.PageToken)
	}
	if err := c.getJSON(ctx, path, q, out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetDraft reads one draft.
func (c *Client) GetDraft(ctx context.Context, address, draftID string) (*Draft, error) {
	out := &Draft{}
	if err := c.getJSON(ctx, draftPath(address, draftID), nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateDraft replaces the fields that are set. CancelSendAt unschedules a
// scheduled draft.
func (c *Client) UpdateDraft(ctx context.Context, address, draftID string, req UpdateDraftRequest) (*Draft, error) {
	out := &Draft{}
	if err := c.sendJSON(ctx, http.MethodPatch, draftPath(address, draftID), req, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteDraft deletes a draft.
func (c *Client) DeleteDraft(ctx context.Context, address, draftID string) error {
	return c.sendJSON(ctx, http.MethodDelete, draftPath(address, draftID), nil, nil, nil)
}

// SendDraft sends a draft now and deletes it. WithIdempotencyKey makes a retry
// return the first result.
func (c *Client) SendDraft(ctx context.Context, address, draftID string, opts ...CallOption) (*SendResult, error) {
	out := &SendResult{}
	if err := c.sendJSON(ctx, http.MethodPost, draftPath(address, draftID)+"/send", nil, opts, out); err != nil {
		return nil, err
	}
	return out, nil
}

func draftPath(address, draftID string) string {
	return "/inboxes/" + escape(address) + "/drafts/" + escape(draftID)
}
