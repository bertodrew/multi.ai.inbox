"use client";

import Link from "next/link";
import { call } from "@/lib/console/client";
import type { ConsoleSession } from "@/lib/console/session";
import { InboxList, MessageList, type Inbox, type Message } from "@/lib/console/schemas";
import { useResource } from "@/lib/console/use-resource";
import { useConsole } from "./ConsoleShell";
import { Button, EmptyState, ErrorState, LoadingState, PageTitle } from "./kit";
import { MessageCard } from "./MessageCard";
import { PermissionNote } from "./PermissionNote";

const MAX_INBOX_PAGES = 5;
const PARALLEL = 4;

async function allInboxes(session: ConsoleSession, signal: AbortSignal): Promise<Inbox[]> {
  const out: Inbox[] = [];
  let token: string | undefined;
  for (let i = 0; i < MAX_INBOX_PAGES; i++) {
    const page = await call(session, "GET /v0/inboxes", { params: {}, query: { limit: 100, page_token: token }, signal }, InboxList);
    out.push(...page.inboxes);
    token = page.next_page_token;
    if (!token) break;
  }
  return out;
}

/** Messages held for review across every inbox, newest first. */
async function heldMessages(session: ConsoleSession, signal: AbortSignal): Promise<Message[]> {
  const inboxes = await allInboxes(session, signal);
  const held: Message[] = [];
  for (let i = 0; i < inboxes.length; i += PARALLEL) {
    const batch = inboxes.slice(i, i + PARALLEL);
    const lists = await Promise.all(
      batch.map((inbox) =>
        call(
          session,
          "GET /v0/inboxes/{inbox}/messages",
          { params: { inbox: inbox.inbox_id }, query: { limit: 100, include_review: true }, signal },
          MessageList,
        ),
      ),
    );
    for (const l of lists) held.push(...l.messages.filter((m) => m.screening === "review"));
  }
  return held.sort((a, b) => b.created_at.localeCompare(a.created_at));
}

export function ReviewView() {
  const { session, can } = useConsole();
  const allowed = can("message_review");
  const queue = useResource(allowed ? "review" : null, (signal) => heldMessages(session, signal), "the review queue");

  const replace = (m: Message) => queue.mutate((cur) => cur.map((x) => (x.id === m.id ? m : x)));

  return (
    <>
      <PageTitle
        title="Review queue"
        intro="Inbound mail the gatekeeper held for a human look. Released messages become visible to agents and fire message.received."
        actions={
          allowed ? (
            <Button onClick={queue.reload} busy={queue.loading && queue.data !== undefined}>
              Refresh
            </Button>
          ) : null
        }
      />
      {!allowed ? (
        <PermissionNote permission="message_review" action="see messages held for review" />
      ) : (
        <>
          {queue.data === undefined ? (
            queue.error ? (
              <ErrorState error={queue.error} onRetry={queue.reload} what="the review queue" />
            ) : (
              <LoadingState label="Checking every inbox for held messages" />
            )
          ) : queue.data.length === 0 ? (
            <EmptyState title="Nothing is waiting for review">New holds appear here as the gatekeeper flags them.</EmptyState>
          ) : (
            <ul className="flex flex-col gap-4">
              {queue.data.map((m) => (
                <li key={m.id}>
                  <p className="mb-1 text-xs text-muted">
                    <span className="font-mono break-all">{m.inbox_id}</span>
                    {" · "}
                    <Link
                      href={`/console/inbox/?address=${encodeURIComponent(m.inbox_id)}&thread=${encodeURIComponent(m.thread_id)}&review=1`}
                      className="text-primary hover:underline"
                    >
                      Open the thread
                    </Link>
                  </p>
                  <MessageCard message={m} onChange={replace} />
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </>
  );
}
