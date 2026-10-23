package aimulti

import (
	"context"
	"net/http"
)

// ListTickets lists the support queue, newest activity first. Filters combine.
func (c *Client) ListTickets(ctx context.Context, f TicketFilter) (*TicketList, error) {
	out := &TicketList{}
	if err := c.getJSON(ctx, "/tickets", f.values(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateTicket opens a ticket on an existing thread. A thread has at most one
// ticket: a second attempt answers 409 ticket_exists.
func (c *Client) CreateTicket(ctx context.Context, req CreateTicketRequest) (*Ticket, error) {
	out := &Ticket{}
	if err := c.sendJSON(ctx, http.MethodPost, "/tickets", req, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// TicketStats reads the queue counters and the median first reply time.
func (c *Client) TicketStats(ctx context.Context) (*TicketStats, error) {
	out := &TicketStats{}
	if err := c.getJSON(ctx, "/tickets/stats", nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetTicket reads one ticket with its timeline and internal notes.
func (c *Client) GetTicket(ctx context.Context, ticketID string) (*TicketDetail, error) {
	out := &TicketDetail{}
	if err := c.getJSON(ctx, "/tickets/"+escape(ticketID), nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateTicket changes state, priority, tags or assignment. A transition the
// state machine does not allow answers 409 invalid_transition.
func (c *Client) UpdateTicket(ctx context.Context, ticketID string, req UpdateTicketRequest) (*Ticket, error) {
	out := &Ticket{}
	if err := c.sendJSON(ctx, http.MethodPatch, "/tickets/"+escape(ticketID), req, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// AddTicketNote adds an internal note. Notes live outside the mailbox and have
// no path to the sender.
func (c *Client) AddTicketNote(ctx context.Context, ticketID, body string) (*TicketNote, error) {
	req := struct {
		Body string `json:"body"`
	}{body}
	out := &TicketNote{}
	if err := c.sendJSON(ctx, http.MethodPost, "/tickets/"+escape(ticketID)+"/notes", req, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ReplyTicket replies to the customer on the ticket thread. It sends through the
// normal reply path, so threading and screening behave as for any reply.
func (c *Client) ReplyTicket(ctx context.Context, ticketID string, req ReplyRequest, opts ...CallOption) (*SendResult, error) {
	out := &SendResult{}
	if err := c.sendJSON(ctx, http.MethodPost, "/tickets/"+escape(ticketID)+"/reply", req, opts, out); err != nil {
		return nil, err
	}
	return out, nil
}
