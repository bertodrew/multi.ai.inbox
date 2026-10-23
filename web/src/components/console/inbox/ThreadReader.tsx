"use client";

// Column 2: the conversation. Every message keeps its own header, sanitized
// body and attachments, and the action bar covers reply, forward, archive and
// delete for the thread as a whole.
import { useEffect, useState, type ReactNode } from "react";
import { useToast } from "@/components/console/Toast";
import { useAction, useResource } from "@/lib/console/use-resource";
import { call } from "@/lib/console/client";
import { Thread, type Message } from "@/lib/console/schemas";
import { useConsole } from "@/components/console/ConsoleShell";
import { Badge, Button, ConfirmButton, EmptyState, ErrorState, LoadingState } from "@/components/console/kit";
import { patchThreadLabels } from "./api-extras";
import { AttachmentList } from "./AttachmentList";
import { Composer } from "./Composer";
import { MessageBody } from "./MessageBody";
import { customLabels, fullTime, isHeld, senderName } from "./mail-util";
import type { ComposeMode, ThreadSummary } from "./types";

function newestMessage(messages: Message[]): Message | undefined {
  return messages.reduce<Message | undefined>(
    (best, message) => (!best || new Date(message.created_at) > new Date(best.created_at) ? message : best),
    undefined,
  );
}

export function ThreadReader({
  threadId,
  includeReview,
  summary,
  onBack,
  onSeen,
  onChanged,
  onArchived,
  onHidden,
  details,
}: {
  threadId: string;
  includeReview: boolean;
  summary: ThreadSummary | undefined;
  onBack: () => void;
  onSeen: (threadId: string, timestamp: string) => void;
  onChanged: () => void;
  onArchived: (threadId: string) => void;
  onHidden: (threadId: string) => void;
  details?: ReactNode;
}) {
  const { session, can } = useConsole();
  const toast = useToast();
  const action = useAction();
  const [mode, setMode] = useState<ComposeMode | null>(null);

  const thread = useResource(
    `thread|${threadId}|${includeReview ? "review" : "visible"}`,
    (signal) =>
      call(
        session,
        "GET /v0/threads/{thread}",
        { params: { thread: threadId }, query: { include_review: includeReview || undefined }, signal },
        Thread,
      ),
    "the thread",
  );

  // Opening a thread marks it read: once in this browser, and once on the
  // server by dropping the unread label, which is what the list reads.
  useEffect(() => {
    if (!thread.data) return;
    const newest = newestMessage(thread.data.messages);
    if (!newest) return;
    onSeen(threadId, newest.created_at);
    if (!can("message_update")) return;
    const unread = thread.data.messages.filter((message) => message.labels.includes("unread"));
    if (unread.length === 0) return;
    let cancelled = false;
    void patchThreadLabels(session, unread, { remove_labels: ["unread"] })
      .then(() => {
        if (!cancelled) onChanged();
      })
      .catch(() => {
        // The local overlay already hides the dot; the label can wait.
      });
    return () => {
      cancelled = true;
    };
  }, [thread.data, threadId, can, session, onChanged, onSeen]);

  const messages = thread.data?.messages ?? [];
  const last = newestMessage(messages);
  const subject = thread.data?.subject || summary?.subject || "(no subject)";
  const sendable = can("message_send") && last !== undefined;

  const archive = () => {
    const targets = messages.length > 0 ? messages : (summary?.messages ?? []);
    if (!can("message_update")) {
      onArchived(threadId);
      toast.info("Archived in this browser", "Adding the archived label needs a key with message_update.");
      return;
    }
    if (targets.length === 0) return;
    void action
      .run(
        "archive",
        () => patchThreadLabels(session, targets, { add_labels: ["archived"] }),
        { success: "Thread archived", failure: "Could not archive the thread" },
      )
      .then((outcome) => {
        if (outcome.ok) onArchived(threadId);
      });
  };

  return (
    <section aria-label="Conversation" className="flex min-h-0 min-w-0 flex-1 flex-col bg-bg">
      <header className="border-b border-border bg-card px-3 py-3">
        <div className="flex items-start gap-1">
          <button
            type="button"
            onClick={onBack}
            className="-ml-1 flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-muted hover:text-fg lg:hidden"
            aria-label="Back to the thread list"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-5 w-5" aria-hidden="true">
              <path d="M15 19l-7-7 7-7" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </button>
          <div className="min-w-0 flex-1">
            <h2 className="font-display text-base font-semibold break-words sm:text-lg">{subject}</h2>
            <p className="mt-0.5 text-xs text-muted">
              {thread.data
                ? `${thread.data.message_count} ${thread.data.message_count === 1 ? "message" : "messages"}`
                : "Loading the conversation"}
            </p>
          </div>
          <Button variant="ghost" className="shrink-0 px-2" onClick={thread.reload} busy={thread.loading} aria-label="Reload the thread">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-4 w-4" aria-hidden="true">
              <path d="M21 12a9 9 0 1 1-3-6.7M21 3v6h-6" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </Button>
        </div>

        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Button
            variant="primary"
            onClick={() => setMode("reply")}
            disabled={!sendable}
            title={can("message_send") ? "Reply in this thread" : "Replying needs message_send"}
          >
            Reply
          </Button>
          <Button
            onClick={() => setMode("forward")}
            disabled={!sendable}
            title={can("message_send") ? "Forward the newest message" : "Forwarding needs message_send"}
          >
            Forward
          </Button>
          <Button onClick={archive} busy={action.busy === "archive"}>
            Archive
          </Button>
          <ConfirmButton
            label="Delete"
            title="Remove this thread from the view?"
            body={
              <>
                The API has no endpoint that deletes a single message yet, so this removes the thread from this browser
                only. It stays on the server and comes back for any other session.
              </>
            }
            confirmLabel="Remove from view"
            onConfirm={async () => {
              onHidden(threadId);
              toast.info("Removed from this view", "The thread is still stored by the API.");
              onBack();
              return true;
            }}
          />
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3 sm:px-4">
        {thread.data === undefined ? (
          thread.error ? (
            <ErrorState error={thread.error} onRetry={thread.reload} what="the thread" />
          ) : (
            <LoadingState label="Loading the conversation" />
          )
        ) : messages.length === 0 ? (
          <EmptyState title="No message you can see">
            Messages held for review stay hidden without the message_review permission. Ask an owner for a key that
            holds it, or release the message from the review queue.
          </EmptyState>
        ) : (
          <div className="flex flex-col gap-3">
            {messages.map((message) => {
              const held = isHeld(message);
              return (
                <article key={message.id} className="rounded-xl border border-border bg-card p-3 sm:p-4">
                  <header>
                    <div className="flex flex-wrap items-baseline gap-2">
                      <span className="text-sm font-semibold break-words">{senderName(message.from)}</span>
                      <span className="min-w-0 truncate font-mono text-[11px] text-muted">{message.from}</span>
                      <span className="ml-auto shrink-0 text-[11px] text-muted">
                        <time dateTime={message.created_at}>{fullTime(message.created_at)}</time>
                      </span>
                    </div>
                    <p className="mt-1 truncate text-[11px] text-muted">
                      to {message.to.join(", ") || "nobody"}
                      {message.cc.length > 0 ? `, cc ${message.cc.join(", ")}` : ""}
                    </p>
                    <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                      <Badge tone={message.direction === "in" ? "neutral" : "good"}>
                        {message.direction === "in" ? "inbound" : message.status}
                      </Badge>
                      {held ? <Badge tone="warn">held for review</Badge> : null}
                      {message.screening === "released" ? <Badge tone="good">released</Badge> : null}
                      {customLabels(message.labels)
                        .slice(0, 4)
                        .map((label) => (
                          <Badge key={label}>{label}</Badge>
                        ))}
                    </div>
                  </header>
                  <MessageBody message={message} />
                  <AttachmentList message={message} />
                </article>
              );
            })}

            {mode && last ? (
              <Composer
                mode={mode}
                message={last}
                onClose={() => setMode(null)}
                onSent={() => {
                  thread.reload();
                  onChanged();
                }}
              />
            ) : null}

            {!mode && last ? (
              <p className="px-1 text-xs text-muted">
                Replying keeps the history in one thread, so an agent reading the API sees the whole conversation.
              </p>
            ) : null}

            {/* On a phone the details column cannot be a third pane, so it
                follows the conversation and scrolls with it. */}
            {details ? <div className="border-t border-border pt-3 lg:hidden">{details}</div> : null}
          </div>
        )}
      </div>
    </section>
  );
}
