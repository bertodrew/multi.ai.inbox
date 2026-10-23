package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bertodrew/multi.ai.inbox/internal/core"
)

// Tickets over MCP: the agent and the human work the same thread. Every call
// writes a timeline row with the key id, so an agent action is attributable.

type createTicketIn struct {
	ThreadID     string   `json:"thread_id" jsonschema:"thread to open a ticket for"`
	Priority     string   `json:"priority,omitempty" jsonschema:"low, normal, high or urgent; default normal"`
	Tags         []string `json:"tags,omitempty"`
	AssigneeKey  string   `json:"assignee_key,omitempty" jsonschema:"assign to this API key (an agent)"`
	AssigneeUser string   `json:"assignee_user,omitempty" jsonschema:"assign to this user id (a person)"`
}

type listTicketsIn struct {
	State     string `json:"state,omitempty" jsonschema:"new, open, pending, resolved or closed"`
	Priority  string `json:"priority,omitempty"`
	Assignee  string `json:"assignee,omitempty" jsonschema:"user id, API key id, or none for unassigned"`
	Inbox     string `json:"inbox,omitempty" jsonschema:"inbox address"`
	Query     string `json:"q,omitempty" jsonschema:"case insensitive text over subject and requester"`
	Limit     int    `json:"limit,omitempty" jsonschema:"1 to 100, default 20"`
	PageToken string `json:"page_token,omitempty"`
}

type ticketIn struct {
	Ticket string `json:"ticket" jsonschema:"ticket id"`
}

type updateTicketIn struct {
	Ticket       string    `json:"ticket" jsonschema:"ticket id"`
	State        *string   `json:"state,omitempty" jsonschema:"new, open, pending, resolved or closed"`
	Priority     *string   `json:"priority,omitempty"`
	Tags         *[]string `json:"tags,omitempty"`
	AssigneeKey  *string   `json:"assignee_key,omitempty" jsonschema:"empty string clears the assignment"`
	AssigneeUser *string   `json:"assignee_user,omitempty"`
}

type assignTicketIn struct {
	Ticket string `json:"ticket" jsonschema:"ticket id"`
	Key    string `json:"assignee_key,omitempty" jsonschema:"assign to this API key (an agent)"`
	User   string `json:"assignee_user,omitempty" jsonschema:"assign to this user id (a person)"`
}

type noteIn struct {
	Ticket string `json:"ticket" jsonschema:"ticket id"`
	Body   string `json:"body" jsonschema:"internal note; never sent to the customer"`
}

type replyTicketIn struct {
	Ticket   string   `json:"ticket" jsonschema:"ticket id"`
	Text     string   `json:"text,omitempty"`
	HTML     string   `json:"html,omitempty"`
	ReplyAll bool     `json:"reply_all,omitempty"`
	Labels   []string `json:"labels,omitempty"`
}

func addTicketTools(s *mcp.Server, g *gate) {
	svc := g.svc
	p := g.c.Principal

	mcp.AddTool(s, &mcp.Tool{Name: "create_ticket",
		Description: "Open a ticket on an existing thread, so a human can follow up on mail the agent handled."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in createTicketIn) (*mcp.CallToolResult, any, error) {
			if err := g.need(core.PermTicketWrite); err != nil {
				return nil, nil, err
			}
			t, err := svc.CreateTicket(ctx, p, core.CreateTicketRequest{
				ThreadID: in.ThreadID, Priority: in.Priority, Tags: in.Tags,
				AssigneeKey: in.AssigneeKey, AssigneeUser: in.AssigneeUser,
			})
			if err == nil {
				g.audit(ctx, "create_ticket", t.ID)
			}
			return nil, t, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "list_tickets",
		Description: "List tickets: the queue, newest activity first. Filter by state, priority, assignee or text."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in listTicketsIn) (*mcp.CallToolResult, any, error) {
			if err := g.need(core.PermTicketRead); err != nil {
				return nil, nil, err
			}
			items, next, err := svc.ListTickets(ctx, p, core.TicketFilter{
				State: in.State, Priority: in.Priority, Assignee: in.Assignee, Inbox: in.Inbox, Query: in.Query,
			}, core.Page{Limit: in.Limit, Token: in.PageToken})
			return nil, listOut("tickets", items, next), err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_ticket",
		Description: "Read one ticket with its timeline and internal notes. The thread id is the mailbox conversation."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ticketIn) (*mcp.CallToolResult, any, error) {
			if err := g.need(core.PermTicketRead); err != nil {
				return nil, nil, err
			}
			t, events, notes, err := svc.GetTicket(ctx, p, in.Ticket)
			if err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"ticket": t, "events": events, "notes": notes}, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "update_ticket",
		Description: "Change state, priority or tags. Transitions are explicit: closing and reopening are audited."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in updateTicketIn) (*mcp.CallToolResult, any, error) {
			if err := g.need(core.PermTicketWrite); err != nil {
				return nil, nil, err
			}
			t, err := svc.UpdateTicket(ctx, p, in.Ticket, core.UpdateTicketRequest{
				State: in.State, Priority: in.Priority, Tags: in.Tags,
				AssigneeKey: in.AssigneeKey, AssigneeUser: in.AssigneeUser,
			})
			if err == nil {
				g.audit(ctx, "update_ticket", in.Ticket)
			}
			return nil, t, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "assign_ticket",
		Description: "Take a ticket or hand it to a human. An agent assigns by its own key id."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in assignTicketIn) (*mcp.CallToolResult, any, error) {
			if err := g.need(core.PermTicketWrite); err != nil {
				return nil, nil, err
			}
			key, user := in.Key, in.User
			t, err := svc.UpdateTicket(ctx, p, in.Ticket, core.UpdateTicketRequest{
				AssigneeKey: &key, AssigneeUser: &user,
			})
			if err == nil {
				g.audit(ctx, "assign_ticket", in.Ticket)
			}
			return nil, t, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "add_ticket_note",
		Description: "Leave an internal note for the human. It is stored separately and can never be emailed to the customer."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in noteIn) (*mcp.CallToolResult, any, error) {
			if err := g.need(core.PermTicketWrite); err != nil {
				return nil, nil, err
			}
			n, err := svc.AddTicketNote(ctx, p, in.Ticket, core.AddNoteRequest{Body: in.Body})
			if err == nil {
				g.audit(ctx, "add_ticket_note", in.Ticket)
			}
			return nil, n, err
		})
	mcp.AddTool(s, &mcp.Tool{Name: "reply_ticket",
		Description: "Answer the customer on the ticket thread through the normal mail path."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in replyTicketIn) (*mcp.CallToolResult, any, error) {
			if err := g.need(core.PermMessageSend); err != nil {
				return nil, nil, err
			}
			res, err := svc.ReplyTicket(ctx, p, in.Ticket, core.ReplyRequest{
				Text: in.Text, HTML: in.HTML, ReplyAll: in.ReplyAll, Labels: in.Labels,
			}, "")
			if err == nil {
				g.audit(ctx, "reply_ticket", in.Ticket)
			}
			return nil, res, err
		})
}
