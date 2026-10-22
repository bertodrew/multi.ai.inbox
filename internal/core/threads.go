package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type Thread struct {
	ThreadID     string     `json:"thread_id"`
	InboxID      string     `json:"inbox_id"`
	Subject      string     `json:"subject"`
	Preview      string     `json:"preview"`
	Labels       []string   `json:"labels"`
	MessageCount int        `json:"message_count"`
	Timestamp    time.Time  `json:"timestamp"`
	CreatedAt    time.Time  `json:"created_at"`
	Messages     []*Message `json:"messages,omitempty"`
}

const threadCols = "t.id, i.address, t.subject, t.preview, t.labels, t.message_count, t.last_message_at, t.created_at"
const threadFrom = " from threads t join inboxes i on i.id = t.inbox_id"

func scanThread(row pgx.Row) (*Thread, error) {
	t := &Thread{}
	err := row.Scan(&t.ThreadID, &t.InboxID, &t.Subject, &t.Preview, &t.Labels, &t.MessageCount, &t.Timestamp, &t.CreatedAt)
	return t, err
}

// ListThreads lists threads of one inbox, or of the whole organization when
// address is empty. A thread shows up only when it has a message the caller
// may see, so a thread started by a held message stays hidden.
func (s *Service) ListThreads(ctx context.Context, orgID, address string, p Page, label string, includeReview bool) ([]*Thread, string, error) {
	cur, err := p.cursor()
	if err != nil {
		return nil, "", err
	}
	var list []*Thread
	err = s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		q := "select " + threadCols + threadFrom + " where exists (select 1 from messages m where m.thread_id = t.id and " +
			viewFor(includeReview).visible() + ")"
		args := []any{p.limit() + 1}
		if address != "" {
			inbox, err := s.inboxTx(ctx, tx, address)
			if err != nil {
				return err
			}
			args = append(args, inbox.ID)
			q += fmt.Sprintf(" and t.inbox_id = $%d", len(args))
		}
		if label != "" {
			args = append(args, label)
			q += fmt.Sprintf(" and $%d = any(t.labels)", len(args))
		}
		if cur != nil {
			args = append(args, cur.At, cur.ID)
			q += fmt.Sprintf(" and (t.last_message_at, t.id) < ($%d, $%d)", len(args)-1, len(args))
		}
		rows, err := tx.Query(ctx, q+" order by t.last_message_at desc, t.id desc limit $1", args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			t, err := scanThread(rows)
			if err != nil {
				return err
			}
			list = append(list, t)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(list) > p.limit() {
		list = list[:p.limit()]
		last := list[len(list)-1]
		next = nextToken(last.Timestamp, last.ThreadID)
	}
	return list, next, nil
}

// GetThread returns the thread with the messages the caller may see, oldest
// first; a thread without any is not found.
func (s *Service) GetThread(ctx context.Context, orgID, threadID string, includeReview bool) (*Thread, error) {
	var out *Thread
	err := s.DB.WithOrg(ctx, orgID, func(tx pgx.Tx) error {
		t, err := scanThread(tx.QueryRow(ctx, "select "+threadCols+threadFrom+" where t.id::text = $1", threadID))
		if errors.Is(err, pgx.ErrNoRows) {
			return NotFound("thread")
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "select "+messageCols+messageFrom+" where m.thread_id = $1 and "+viewFor(includeReview).visible()+
			" order by m.created_at, m.id", t.ThreadID)
		if err != nil {
			return err
		}
		for rows.Next() {
			m, err := scanMessage(rows)
			if err != nil {
				rows.Close()
				return err
			}
			t.Messages = append(t.Messages, m)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(t.Messages) == 0 {
			return NotFound("thread")
		}
		for _, m := range t.Messages {
			if err := loadAttachments(ctx, tx, m); err != nil {
				return err
			}
		}
		out = t
		return nil
	})
	return out, err
}
