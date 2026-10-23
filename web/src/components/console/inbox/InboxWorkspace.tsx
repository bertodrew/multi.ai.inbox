"use client";

// The three pane webmail: threads, the conversation, and details with labels.
// On a phone it collapses to one pane at a time with a back button, because a
// 390px screen cannot show three columns honestly.
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useToast } from "@/components/console/Toast";
import { useResource } from "@/lib/console/use-resource";
import { call, errorMessage } from "@/lib/console/client";
import { InboxList } from "@/lib/console/schemas";
import { useConsole } from "@/components/console/ConsoleShell";
import { EmptyState, ErrorState, LoadingState } from "@/components/console/kit";
import { patchThreadLabels } from "./api-extras";
import { DetailsPane } from "./DetailsPane";
import { ThreadListPane } from "./ThreadListPane";
import { ThreadReader } from "./ThreadReader";
import { useMailbox } from "./useMailbox";
import { useMailView } from "./state";
import type { ThreadSummary } from "./types";

function inboxHref(address: string): string {
  return `/console/inbox/?address=${encodeURIComponent(address)}`;
}

export function InboxWorkspace() {
  const params = useSearchParams();
  const router = useRouter();
  const { session, can } = useConsole();

  const address = params.get("address") ?? "";
  const label = params.get("label") ?? "";
  const review = params.get("review") === "1" && can("message_review");
  const initialThread = params.get("thread") ?? "";

  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState("");
  const [showArchived, setShowArchived] = useState(false);
  const toast = useToast();
  const view = useMailView(address);
  const {
    isRead: cachedRead,
    isArchived: cachedArchived,
    isHidden: cachedHidden,
    markRead: rememberRead,
    markUnread: forgetRead,
    setArchived: rememberArchived,
    setHidden: rememberHidden,
  } = view;
  const mailbox = useMailbox({
    session,
    inbox: address,
    query,
    label,
    includeReview: review,
    enabled: address !== "" && can("message_read"),
  });
  const { reload } = mailbox;

  const inboxes = useResource(
    can("inbox_read") ? "inboxes|picker" : null,
    (signal) => call(session, "GET /v0/inboxes", { params: {}, query: { limit: 100 }, signal }, InboxList),
    "your inboxes",
  );

  // Apply the thread from the link once, after hydration has real params.
  const appliedInitial = useRef(false);
  useEffect(() => {
    if (appliedInitial.current || !initialThread) return;
    appliedInitial.current = true;
    setSelected(initialThread);
  }, [initialThread]);

  // A different inbox means a different list; do not keep a stale selection.
  const previousAddress = useRef(address);
  useEffect(() => {
    if (previousAddress.current === address) return;
    previousAddress.current = address;
    setSelected("");
    setQuery("");
  }, [address]);

  const readThread = useCallback(
    (thread: ThreadSummary) => !thread.unread || cachedRead(thread.thread_id, thread.timestamp),
    [cachedRead],
  );

  const isArchived = useCallback(
    (thread: ThreadSummary) => cachedArchived(thread.thread_id) || thread.labels.includes("archived"),
    [cachedArchived],
  );

  const threads = useMemo(
    () => mailbox.threads.filter((thread) => !cachedHidden(thread.thread_id) && (showArchived || !isArchived(thread))),
    [mailbox.threads, cachedHidden, showArchived, isArchived],
  );

  const unreadCount = threads.filter((thread) => !readThread(thread)).length;
  const archivedCount = mailbox.threads.filter(isArchived).length;

  const openThread = useCallback(
    (thread: ThreadSummary) => {
      setSelected(thread.thread_id);
      rememberRead(thread.thread_id, thread.timestamp);
    },
    [rememberRead],
  );

  const onSeen = useCallback(
    (threadId: string, timestamp: string) => rememberRead(threadId, timestamp),
    [rememberRead],
  );

  const onArchived = useCallback(
    (threadId: string) => {
      rememberArchived(threadId, true);
      reload();
    },
    [rememberArchived, reload],
  );

  const onUnarchive = useCallback(
    (threadId: string) => {
      rememberArchived(threadId, false);
      reload();
    },
    [rememberArchived, reload],
  );

  const onHidden = useCallback(
    (threadId: string) => {
      rememberHidden(threadId, true);
      setSelected("");
    },
    [rememberHidden],
  );

  // The address in the link may not be one the key can list (a review link
  // can point at it), so keep it selectable instead of silently swapping it.
  const inboxOptions = useMemo(() => {
    const listed = inboxes.data?.inboxes.map((inbox) => inbox.inbox_id) ?? [];
    return listed.includes(address) ? listed : [address, ...listed];
  }, [inboxes.data, address]);

  const selectedSummary = useMemo(
    () => mailbox.threads.find((thread) => thread.thread_id === selected),
    [mailbox.threads, selected],
  );

  const markUnread = useCallback(() => {
    if (!selectedSummary || !can("message_update")) return;
    void patchThreadLabels(session, selectedSummary.messages, { add_labels: ["unread"] })
      .then(() => {
        forgetRead(selectedSummary.thread_id);
        reload();
      })
      .catch((error: unknown) => toast.error("Could not mark the thread unread", errorMessage(error)));
    // toast is stable from the provider; keeping it out of the deps below
    // would need an extra ref, so it is listed.
  }, [selectedSummary, can, session, forgetRead, reload, toast]);

  if (!address) {
    return (
      <div>
        <h1 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">Inbox</h1>
        <p className="mt-2 max-w-2xl text-muted">
          Read, reply and label the mail your agents receive. Pick an inbox to open its threads.
        </p>
        <div className="mt-5">
          {!can("inbox_read") ? (
            <div role="note" className="rounded-xl border border-border border-l-4 border-l-accent bg-card px-4 py-4">
              <p className="font-medium">This key cannot list inboxes</p>
              <p className="mt-1 text-sm text-muted">
                The inbox picker needs the inbox_read permission. Open a thread from the review queue or a link that
                already names the inbox, or ask an owner for a scoped key with inbox_read and message_read.
              </p>
            </div>
          ) : inboxes.data === undefined ? (
            inboxes.error ? (
              <ErrorState error={inboxes.error} onRetry={inboxes.reload} what="your inboxes" />
            ) : (
              <LoadingState label="Loading your inboxes" />
            )
          ) : inboxes.data.inboxes.length === 0 ? (
            <EmptyState title="No inbox yet">
              Create an inbox first, then its mail shows up here. Every message becomes a thread you can read, label and
              answer, and your agents read the same threads over the API.
            </EmptyState>
          ) : (
            <ul className="grid gap-2 sm:grid-cols-2">
              {inboxes.data.inboxes.map((inbox) => (
                <li key={inbox.id}>
                  <Link
                    href={inboxHref(inbox.inbox_id)}
                    className="flex min-h-14 flex-col justify-center rounded-xl border border-border bg-card px-4 py-2 hover:border-fg/40"
                  >
                    <span className="truncate text-sm font-medium">{inbox.display_name || inbox.inbox_id}</span>
                    <span className="truncate font-mono text-xs text-muted">{inbox.inbox_id}</span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    );
  }

  if (!can("message_read")) {
    return (
      <div>
        <h1 className="font-display text-2xl font-semibold tracking-tight break-words">{address}</h1>
        <div role="note" className="mt-4 rounded-xl border border-border border-l-4 border-l-accent bg-card px-4 py-4">
          <p className="font-medium">This key cannot read mail</p>
          <p className="mt-1 text-sm text-muted">
            Reading threads needs the message_read permission. Ask an owner of a full access key to create a scoped key
            with it on the API keys page.
          </p>
        </div>
      </div>
    );
  }

  // Details render twice: in the third column on desktop and under the reader
  // on a phone. Separate id prefixes keep the two label forms distinct.
  const renderDetails = (idPrefix: string) => (
    <DetailsPane
      key={idPrefix}
      idPrefix={idPrefix}
      address={address}
      summary={selectedSummary}
      archived={selectedSummary ? isArchived(selectedSummary) : false}
      unread={selectedSummary ? !readThread(selectedSummary) : false}
      onChanged={reload}
      onMarkUnread={markUnread}
      onUnarchive={() => {
        if (selectedSummary) onUnarchive(selectedSummary.thread_id);
      }}
    />
  );

  return (
    <div className="flex min-h-0 flex-col">
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
        <div className="min-w-0">
          <h1 className="font-display truncate text-xl font-semibold tracking-tight sm:text-2xl">{address}</h1>
          <p className="mt-0.5 flex flex-wrap gap-x-3 text-xs text-muted">
            <Link href="/console/" className="text-primary hover:underline">
              All inboxes
            </Link>
            <Link
              href={`/console/compose/?from=${encodeURIComponent(address)}`}
              className="text-primary hover:underline"
            >
              Compose from this inbox
            </Link>
          </p>
        </div>
        {inboxOptions.length > 1 ? (
          <label className="flex items-center gap-2 text-xs text-muted">
            <span>Inbox</span>
            <select
              value={address}
              onChange={(event) => router.replace(inboxHref(event.target.value))}
              className="min-h-11 max-w-[14rem] rounded-lg border border-border bg-card px-2 text-sm text-fg"
            >
              {inboxOptions.map((option) => (
                <option key={option} value={option}>
                  {option}
                </option>
              ))}
            </select>
          </label>
        ) : null}
      </div>

      <div className="flex h-[calc(100dvh-11rem)] min-h-[32rem] flex-col overflow-hidden rounded-xl border border-border bg-card lg:grid lg:h-[calc(100dvh-10rem)] lg:grid-cols-[minmax(16rem,20rem)_minmax(0,1fr)_minmax(14rem,17rem)] lg:divide-x lg:divide-border">
        <div className={`${selected ? "hidden" : "flex"} min-h-0 min-w-0 flex-col lg:flex`}>
          <ThreadListPane
            address={address}
            query={query}
            onQuery={setQuery}
            label={label}
            onClearLabel={() => router.replace(inboxHref(address))}
            threads={threads}
            loading={mailbox.loading}
            scanning={mailbox.scanning}
            error={mailbox.error}
            hasMore={mailbox.hasMore}
            loadingMore={mailbox.loadingMore}
            moreError={mailbox.moreError}
            onLoadMore={mailbox.loadMore}
            onRetry={reload}
            selectedId={selected}
            onSelect={openThread}
            isRead={readThread}
            showArchived={showArchived}
            onToggleArchived={() => setShowArchived((value) => !value)}
            scanned={mailbox.scanned}
            unreadCount={unreadCount}
            archivedCount={archivedCount}
          />
        </div>

        <div className={`${selected ? "flex" : "hidden"} min-h-0 min-w-0 flex-col lg:flex`}>
          {selected ? (
            <ThreadReader
              key={selected}
              threadId={selected}
              includeReview={review}
              summary={selectedSummary}
              onBack={() => setSelected("")}
              onSeen={onSeen}
              onChanged={reload}
              onArchived={onArchived}
              onHidden={onHidden}
              details={renderDetails("thread-mobile")}
            />
          ) : (
            <div className="flex flex-1 items-center justify-center p-4">
              <div className="max-w-sm text-center">
                <p className="font-medium">Pick a thread</p>
                <p className="mt-2 text-sm text-muted">
                  The conversation opens here with its sanitized HTML, attachments and one action bar for reply, forward,
                  archive and delete.
                </p>
              </div>
            </div>
          )}
        </div>

        <div className="hidden min-h-0 min-w-0 flex-col lg:flex">{renderDetails("thread")}</div>
      </div>
    </div>
  );
}
