package aimulti

import "net/url"

// ListOptions are the parameters the list operations share.
type ListOptions struct {
	// Limit is the page size, 1 to 100. Zero means the API default of 20.
	Limit int
	// PageToken continues from a previous page.
	PageToken string
	// Label filters messages and threads by label.
	Label string
	// IncludeReview also returns messages held for security review. It needs
	// the message_review permission and is meant for a human console.
	IncludeReview bool
}

func (o ListOptions) values() url.Values {
	v := url.Values{}
	if o.Limit > 0 {
		v.Set("limit", itoa(o.Limit))
	}
	if o.PageToken != "" {
		v.Set("page_token", o.PageToken)
	}
	if o.Label != "" {
		v.Set("label", o.Label)
	}
	if o.IncludeReview {
		v.Set("include_review", "true")
	}
	return v
}

// TicketFilter are the support queue filters. Empty fields are not sent.
type TicketFilter struct {
	State     string
	Priority  string
	Assignee  string // a user id, an API key id, or "none"
	Inbox     string // an inbox address
	Query     string // case insensitive over subject and requester
	Limit     int
	PageToken string
}

func (f TicketFilter) values() url.Values {
	v := url.Values{}
	for k, val := range map[string]string{
		"state": f.State, "priority": f.Priority, "assignee": f.Assignee,
		"inbox": f.Inbox, "q": f.Query, "page_token": f.PageToken,
	} {
		if val != "" {
			v.Set(k, val)
		}
	}
	if f.Limit > 0 {
		v.Set("limit", itoa(f.Limit))
	}
	return v
}

// EventOptions select which events a stream delivers.
type EventOptions struct {
	// Types is a list of event types such as "message.received". All events
	// when empty.
	Types []string
	// Inbox limits the stream to one inbox address.
	Inbox string
	// LastEventID resumes after an event id, for a reconnect.
	LastEventID int64
}

func (o EventOptions) values() url.Values {
	v := url.Values{}
	if len(o.Types) > 0 {
		v.Set("types", join(o.Types, ","))
	}
	if o.Inbox != "" {
		v.Set("inbox", o.Inbox)
	}
	if o.LastEventID > 0 {
		v.Set("last_event_id", itoa64(o.LastEventID))
	}
	return v
}
