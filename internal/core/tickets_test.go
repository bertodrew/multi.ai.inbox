package core

import (
	"testing"
	"time"
)

// The state machine is the contract: nothing moves silently and a closed
// ticket only reopens. Anything else is refused with invalid_transition.
func TestTicketTransitions(t *testing.T) {
	allowed := [][2]string{
		{TicketNew, TicketOpen}, {TicketNew, TicketPending}, {TicketNew, TicketResolved}, {TicketNew, TicketClosed},
		{TicketOpen, TicketPending}, {TicketOpen, TicketResolved}, {TicketOpen, TicketClosed},
		{TicketPending, TicketOpen}, {TicketPending, TicketResolved}, {TicketPending, TicketClosed},
		{TicketResolved, TicketOpen}, {TicketResolved, TicketClosed},
		{TicketClosed, TicketOpen},
	}
	for _, c := range allowed {
		if !transitionAllowed(c[0], c[1]) {
			t.Errorf("%s -> %s should be allowed", c[0], c[1])
		}
	}
	refused := [][2]string{
		{TicketClosed, TicketPending}, {TicketClosed, TicketResolved}, {TicketClosed, TicketNew},
		{TicketResolved, TicketPending}, {TicketResolved, TicketNew}, {TicketOpen, TicketNew},
		{"nonsense", TicketOpen}, {TicketNew, "nonsense"},
	}
	for _, c := range refused {
		if transitionAllowed(c[0], c[1]) {
			t.Errorf("%s -> %s should be refused", c[0], c[1])
		}
	}
	for _, s := range []string{TicketNew, TicketOpen, TicketPending, TicketResolved, TicketClosed} {
		if !validState(s) {
			t.Errorf("%q should be a valid state", s)
		}
		if !transitionAllowed(s, s) {
			t.Errorf("%s -> %s (no change) should be allowed", s, s)
		}
	}
}

// Business hours: Monday to Friday, 09:00 to 18:00 CET, so a 4 hour promise
// made at 17:00 on a Friday lands on Monday morning, not the same evening.
func TestBusinessHours(t *testing.T) {
	at := func(day int, hour, min int) time.Time {
		return time.Date(2026, 10, day, hour, min, 0, 0, cetZone)
	}
	cases := []struct {
		name string
		from time.Time
		d    time.Duration
		want time.Time
	}{
		{"inside the day", at(19, 10, 0), 4 * time.Hour, at(19, 14, 0)},
		{"spills to the next morning", at(19, 17, 0), 4 * time.Hour, at(20, 12, 0)},
		{"across the weekend", at(23, 17, 0), 4 * time.Hour, at(26, 12, 0)}, // Fri 17:00 + 4h -> Mon 12:00
		{"before opening", at(20, 7, 0), 2 * time.Hour, at(20, 11, 0)},
	}
	for _, c := range cases {
		if got := addBusiness(c.from, c.d); !got.Equal(c.want) {
			t.Errorf("%s: got %s, want %s", c.name, got.Format(time.RFC3339), c.want.Format(time.RFC3339))
		}
	}
}

// The SLA follows the plan, and urgency shortens it. A free organization gets
// no promise at all rather than a promise nobody keeps.
func TestSLADue(t *testing.T) {
	now := time.Date(2026, 10, 19, 10, 0, 0, 0, cetZone) // Monday 10:00
	if first, res := slaDue(now, "free", PriorityNormal); first != nil || res != nil {
		t.Fatalf("free plan must not promise an SLA, got %v / %v", first, res)
	}
	first, _ := slaDue(now, "team", PriorityNormal)
	if first == nil || !first.Equal(now.Add(4*time.Hour)) {
		t.Fatalf("team first reply: got %v, want Monday 14:00", first)
	}
	urgent, _ := slaDue(now, "team", PriorityUrgent)
	if urgent == nil || !urgent.Equal(now.Add(time.Hour)) {
		t.Fatalf("urgent first reply: got %v, want Monday 11:00", urgent)
	}
	if !urgent.Before(*first) {
		t.Fatal("urgent must come before normal")
	}
}

// Breaching is what paints the row red: only an unanswered, still open ticket
// can breach, and only once the deadline has passed.
func TestTicketBreaching(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	cases := []struct {
		name string
		t    Ticket
		want bool
	}{
		{"just past due", Ticket{State: TicketNew, DueFirstReply: &past}, true},
		{"open and past due", Ticket{State: TicketOpen, DueFirstReply: &past}, true},
		{"already answered", Ticket{State: TicketOpen, DueFirstReply: &past, FirstReplyAt: &past}, false},
		{"pending is our turn", Ticket{State: TicketPending, DueFirstReply: &past}, false},
		{"not yet due", Ticket{State: TicketNew, DueFirstReply: &future}, false},
		{"no promise", Ticket{State: TicketNew}, false},
	}
	for _, c := range cases {
		if got := c.t.Breaching(now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// A ticket is a thread: the queue must not let a key change state and post a
// customer reply with one permission. This is the check that keeps the two
// rights separate in the permission table.
func TestTicketPermissionsAreSeparate(t *testing.T) {
	if PermTicketWrite == PermMessageSend || PermTicketRead == PermMessageSend {
		t.Fatal("ticket_rights must be distinct from message_send")
	}
	seen := map[string]bool{}
	for _, p := range AllPermissions {
		if seen[p] {
			t.Fatalf("duplicate permission %q", p)
		}
		seen[p] = true
	}
	for _, p := range []string{PermTicketRead, PermTicketWrite} {
		if !seen[p] {
			t.Fatalf("%q missing from AllPermissions", p)
		}
	}
	// A member can read the queue but not change it.
	member := PermissionsForRole("member")
	if !hasPerm(member, PermTicketRead) {
		t.Error("member should read tickets")
	}
	if hasPerm(member, PermTicketWrite) {
		t.Error("member must not change tickets")
	}
}

func hasPerm(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
