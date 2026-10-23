// Command example shows the shape of a small agent loop with the aimulti
// client. It is compiled by `go build ./...` so the snippet in the README cannot
// rot. Run it with MAI_API_KEY set and, for a deployment other than production,
// MAI_BASE_URL.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	aimulti "github.com/bertodrew/multi.ai.inbox/sdk/go"
)

func main() {
	key := os.Getenv("MAI_API_KEY")
	if key == "" {
		log.Fatal("set MAI_API_KEY to an API key from the console")
	}
	opts := []aimulti.Option{}
	if base := os.Getenv("MAI_BASE_URL"); base != "" {
		opts = append(opts, aimulti.WithBaseURL(base))
	}
	client := aimulti.NewClient(key, opts...)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	me, err := client.Me(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("organization %s on the %s plan\n", me.OrganizationID, me.Plan)

	inbox, err := client.CreateInbox(ctx, aimulti.CreateInboxRequest{
		Username: "support-agent",
		ClientID: "support-agent", // repeating this returns the same inbox
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("inbox %s\n", inbox.InboxID)

	threads, err := client.ListThreads(ctx, inbox.InboxID, aimulti.ListOptions{Limit: 5})
	if err != nil {
		log.Fatal(err)
	}
	for _, thread := range threads.Threads {
		fmt.Printf("%s  %s\n", thread.ThreadID, thread.Subject)
	}

	// A retry with the same idempotency key returns the first result instead of
	// sending a second message.
	result, err := client.Send(ctx, inbox.InboxID, aimulti.SendRequest{
		To:      []string{"person@example.com"},
		Subject: "Invoice approved",
		Text:    "The invoice is approved and will be paid this week.",
	}, aimulti.WithIdempotencyKey("invoice-42-approval"))
	if err != nil {
		var apiErr *aimulti.Error
		if errors.As(err, &apiErr) && aimulti.IsCode(err, "rate_limited") {
			fmt.Printf("rate limited, retry in %s\n", aimulti.RetryAfter(err))
			return
		}
		log.Fatal(err)
	}
	fmt.Printf("queued %s in thread %s\n", result.MessageID, result.ThreadID)
}
