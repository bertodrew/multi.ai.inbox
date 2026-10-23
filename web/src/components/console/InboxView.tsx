"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState, type FormEvent } from "react";
import { call } from "@/lib/console/client";
import { LabelFilter, ReplyInput, SendResult, Thread, ThreadList, type Message } from "@/lib/console/schemas";
import { useAction, useResource } from "@/lib/console/use-resource";
import { useConsole } from "./ConsoleShell";
import { Badge, Button, EmptyState, ErrorState, Field, inputClass, LoadingState, PageTitle, When } from "./kit";
import { MessageCard } from "./MessageCard";

const link = (address: string, thread?: string, label?: string, review?: boolean) => {
  const q = new URLSearchParams({ address });
  if (thread) q.set("thread", thread);
  if (label) q.set("label", label);
  if (review) q.set("review", "1");
  return `/console/inbox/?${q.toString()}`;
};

export function InboxView() {
  const params = useSearchParams();
  const address = params.get("address") ?? "";
  const threadId = params.get("thread") ?? "";
  const label = params.get("label") ?? "";
  const { can } = useConsole();
  const review = params.get("review") === "1" && can("message_review");

  if (!address) {
    return (
      <EmptyState title="No inbox selected">
        <Link href="/console/" className="text-primary underline">
          Pick an inbox
        </Link>{" "}
        to see its threads.
      </EmptyState>
    );
  }

  return (
    <>
      <PageTitle
        title={address}
        intro={
          <>
            <Link href="/console/" className="text-primary hover:underline">
              All inboxes
            </Link>
            {" · "}
            <Link href={`/console/compose/?from=${encodeURIComponent(address)}`} className="text-primary hover:underline">
              Compose from this inbox
            </Link>
          </>
        }
      />
      <div className="grid gap-6 lg:grid-cols-[minmax(0,20rem)_minmax(0,1fr)]">
        <div className={threadId ? "hidden lg:block" : ""}>
          <ThreadColumn address={address} activeThread={threadId} label={label} review={review} />
        </div>
        <div className={threadId ? "" : "hidden lg:block"}>
          {threadId ? (
            <>
              <Link
                href={link(address, undefined, label, review)}
                className="mb-3 inline-flex min-h-11 items-center text-sm text-primary hover:underline lg:hidden"
              >
                Back to threads
              </Link>
              <ThreadDetail key={`${threadId}|${review}`} threadId={threadId} review={review} />
            </>
          ) : (
            <EmptyState title="No thread open">Pick a thread on the left to read it.</EmptyState>
          )}
        </div>
      </div>
    </>
  );
}

function ThreadColumn({
  address,
  activeThread,
  label,
  review,
}: {
  address: string;
  activeThread: string;
  label: string;
  review: boolean;
}) {
  const { session, can } = useConsole();
  const router = useRouter();
  const action = useAction();
  const [filter, setFilter] = useState(label);
  const [filterError, setFilterError] = useState<string>();
  const threads = useResource(
    `threads|${address}|${label}|${review}`,
    (signal) =>
      call(
        session,
        "GET /v0/inboxes/{inbox}/threads",
        { params: { inbox: address }, query: { limit: 50, label, include_review: review || undefined }, signal },
        ThreadList,
      ),
    "threads",
  );

  const loadMore = async (token: string) => {
    const out = await action.run(
      "more",
      () =>
        call(
          session,
          "GET /v0/inboxes/{inbox}/threads",
          { params: { inbox: address }, query: { limit: 50, label, include_review: review || undefined, page_token: token } },
          ThreadList,
        ),
      { failure: "Could not load more threads" },
    );
    if (out.ok) {
      threads.mutate((cur) => ({ ...out.value, threads: [...cur.threads, ...out.value.threads] }));
    }
  };

  const applyFilter = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    // Any label is a valid filter, system labels included.
    const parsed = LabelFilter.safeParse(filter);
    if (!parsed.success) {
      setFilterError(parsed.error.issues[0]?.message);
      return;
    }
    setFilterError(undefined);
    router.push(link(address, undefined, parsed.data || undefined, review));
  };

  return (
    <div>
      <form onSubmit={applyFilter} noValidate className="flex gap-2">
        <label htmlFor="label-filter" className="sr-only">
          Filter by label
        </label>
        <input
          id="label-filter"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Filter by label"
          autoCapitalize="none"
          spellCheck={false}
          className={inputClass}
        />
        <Button type="submit">Filter</Button>
      </form>
      {filterError ? (
        <p role="alert" className="mt-1 text-sm text-danger">
          {filterError}
        </p>
      ) : null}
      {can("message_review") ? (
        <Link
          href={link(address, activeThread || undefined, label || undefined, !review)}
          className="mt-2 inline-flex min-h-11 items-center text-sm text-primary hover:underline"
        >
          {review ? "Hide messages held for review" : "Include messages held for review"}
        </Link>
      ) : null}

      <div className="mt-3">
        {threads.data === undefined ? (
          threads.error ? (
            <ErrorState error={threads.error} onRetry={threads.reload} what="threads" />
          ) : (
            <LoadingState label="Loading threads" />
          )
        ) : threads.data.threads.length === 0 ? (
          <EmptyState title={label ? `No threads labelled ${label}` : "No threads yet"}>
            {label ? "Clear the filter to see every thread." : "Mail sent to this address shows up here."}
          </EmptyState>
        ) : (
          <>
            <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-card">
              {threads.data.threads.map((t) => (
                <li key={t.thread_id}>
                  <Link
                    href={link(address, t.thread_id, label || undefined, review)}
                    aria-current={t.thread_id === activeThread ? "true" : undefined}
                    className={`block p-3 hover:bg-bg ${t.thread_id === activeThread ? "bg-bg" : ""}`}
                  >
                    <span className="flex items-baseline justify-between gap-2">
                      <span className="truncate text-sm font-medium">{t.subject || "(no subject)"}</span>
                      <span className="shrink-0 text-xs text-muted">{t.message_count}</span>
                    </span>
                    <span className="mt-1 line-clamp-2 block text-xs text-muted">{t.preview}</span>
                    <span className="mt-1 flex flex-wrap items-center gap-1 text-xs text-muted">
                      <When iso={t.timestamp} />
                      {t.labels
                        .filter((l) => l !== "received" && l !== "sent")
                        .slice(0, 3)
                        .map((l) => (
                          <Badge key={l}>{l}</Badge>
                        ))}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
            {threads.data.next_page_token ? (
              <Button
                className="mt-3 w-full"
                busy={action.busy === "more"}
                onClick={() => threads.data?.next_page_token && loadMore(threads.data.next_page_token)}
              >
                Load more
              </Button>
            ) : null}
          </>
        )}
      </div>
    </div>
  );
}

function ThreadDetail({ threadId, review }: { threadId: string; review: boolean }) {
  const { session, can } = useConsole();
  const thread = useResource(
    `thread|${threadId}|${review}`,
    (signal) =>
      call(session, "GET /v0/threads/{thread}", { params: { thread: threadId }, query: { include_review: review || undefined }, signal }, Thread),
    "the thread",
  );

  const replace = (m: Message) =>
    thread.mutate((cur) => ({ ...cur, messages: cur.messages.map((x) => (x.id === m.id ? m : x)) }));

  if (thread.data === undefined) {
    return thread.error ? (
      <ErrorState error={thread.error} onRetry={thread.reload} what="the thread" />
    ) : (
      <LoadingState label="Loading the thread" />
    );
  }

  const t = thread.data;
  const last = t.messages.at(-1);
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-display text-xl font-semibold break-words">{t.subject || "(no subject)"}</h2>
        <Button onClick={thread.reload} busy={thread.loading}>
          Refresh
        </Button>
      </div>
      {t.messages.length === 0 ? (
        <EmptyState title="No messages you can see">
          Messages still being screened, or held for review without the message_review permission, are hidden.
        </EmptyState>
      ) : (
        t.messages.map((m) => <MessageCard key={m.id} message={m} onChange={replace} />)
      )}
      {last ? (
        can("message_send") ? (
          <ReplyForm last={last} onSent={thread.reload} />
        ) : (
          <p className="text-sm text-muted">Replying needs a key with the message_send permission.</p>
        )
      ) : null}
    </div>
  );
}

function ReplyForm({ last, onSent }: { last: Message; onSent: () => void }) {
  const { session } = useConsole();
  const action = useAction();
  const [text, setText] = useState("");
  const [replyAll, setReplyAll] = useState(false);
  const [error, setError] = useState<string>();
  // One key per draft: a retry after a network error never sends twice.
  const [idempotencyKey, setIdempotencyKey] = useState(() => crypto.randomUUID());

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const parsed = ReplyInput.safeParse({ text, reply_all: replyAll });
    if (!parsed.success) {
      setError(parsed.error.issues[0]?.message);
      return;
    }
    setError(undefined);
    const out = await action.run(
      "reply",
      () =>
        call(
          session,
          "POST /v0/inboxes/{inbox}/messages/{message}/reply",
          { params: { inbox: last.inbox_id, message: last.id }, body: parsed.data, idempotencyKey },
          SendResult,
        ),
      { success: "Reply queued for sending", failure: "Could not send the reply" },
    );
    if (out.ok) {
      setText("");
      setIdempotencyKey(crypto.randomUUID());
      onSent();
    }
  };

  return (
    <form onSubmit={submit} noValidate className="rounded-xl border border-border bg-card p-4 sm:p-5">
      <Field label={`Reply to ${last.direction === "in" ? last.from : last.to.join(", ")}`} htmlFor="reply-text" error={error}>
        <textarea id="reply-text" rows={5} value={text} onChange={(e) => setText(e.target.value)} className={inputClass} />
      </Field>
      <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <label className="flex min-h-11 items-center gap-2 text-sm">
          <input type="checkbox" checked={replyAll} onChange={(e) => setReplyAll(e.target.checked)} className="h-4 w-4" />
          Reply to all
        </label>
        <Button type="submit" variant="primary" busy={action.busy === "reply"}>
          Send reply
        </Button>
      </div>
    </form>
  );
}
