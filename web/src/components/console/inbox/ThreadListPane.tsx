"use client";

// Column 1: search, filters and the virtualized thread list with its loading,
// error and empty states.
import { useEffect, useRef, useState } from "react";
import { Button, ErrorState, Spinner, inputClass } from "@/components/console/kit";
import { errorMessage } from "@/lib/console/client";
import { VirtualThreadList } from "./VirtualThreadList";
import type { ThreadSummary } from "./types";

function Skeleton() {
  return (
    <div role="status" aria-live="polite" className="flex flex-col gap-0">
      <span className="sr-only">Loading threads</span>
      {[0, 1, 2, 3, 4, 5].map((row) => (
        <div key={row} className="flex animate-pulse items-start gap-2.5 border-b border-border px-3 py-3" aria-hidden="true">
          <span className="h-8 w-8 shrink-0 rounded-full bg-border/70" />
          <span className="flex-1">
            <span className="block h-3 w-1/3 rounded bg-border/70" />
            <span className="mt-2 block h-3 w-3/4 rounded bg-border/50" />
            <span className="mt-2 block h-3 w-1/2 rounded bg-border/40" />
          </span>
        </div>
      ))}
    </div>
  );
}

export function ThreadListPane({
  address,
  query,
  onQuery,
  label,
  onClearLabel,
  threads,
  loading,
  scanning,
  error,
  hasMore,
  loadingMore,
  moreError,
  onLoadMore,
  onRetry,
  selectedId,
  onSelect,
  isRead,
  showArchived,
  onToggleArchived,
  scanned,
  unreadCount,
  archivedCount,
}: {
  address: string;
  query: string;
  onQuery: (query: string) => void;
  label: string;
  onClearLabel: () => void;
  threads: ThreadSummary[];
  loading: boolean;
  scanning: boolean;
  error: unknown;
  hasMore: boolean;
  loadingMore: boolean;
  moreError: unknown;
  onLoadMore: () => void;
  onRetry: () => void;
  selectedId: string;
  onSelect: (thread: ThreadSummary) => void;
  isRead: (thread: ThreadSummary) => boolean;
  showArchived: boolean;
  onToggleArchived: () => void;
  scanned: number;
  unreadCount: number;
  archivedCount: number;
}) {
  const [draft, setDraft] = useState(query);
  const emitted = useRef(query);

  useEffect(() => {
    if (query === emitted.current) return;
    emitted.current = query;
    setDraft(query);
  }, [query]);

  useEffect(() => {
    const timer = setTimeout(() => {
      if (draft === emitted.current) return;
      emitted.current = draft;
      onQuery(draft);
    }, 300);
    return () => clearTimeout(timer);
  }, [draft, onQuery]);

  const searching = query.trim() !== "";

  return (
    <section aria-label="Threads" className="flex min-h-0 min-w-0 flex-1 flex-col bg-card">
      <div className="border-b border-border px-3 py-3">
        <div className="flex items-center gap-2">
          <div className="relative min-w-0 flex-1">
            <label htmlFor="mail-search" className="sr-only">
              Search mail
            </label>
            <input
              id="mail-search"
              type="text"
              inputMode="search"
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Escape") setDraft("");
                if (event.key === "Enter") {
                  emitted.current = draft;
                  onQuery(draft);
                }
              }}
              placeholder="Search sender, subject, text"
              autoComplete="off"
              spellCheck={false}
              className={`${inputClass} pr-11`}
            />
            {draft ? (
              <button
                type="button"
                onClick={() => {
                  setDraft("");
                  emitted.current = "";
                  onQuery("");
                }}
                className="absolute top-0 right-0 flex h-11 w-11 items-center justify-center text-muted hover:text-fg"
                aria-label="Clear search"
              >
                <span aria-hidden="true" className="text-lg leading-none">
                  x
                </span>
              </button>
            ) : null}
          </div>
          <Button variant="ghost" className="px-3" onClick={onRetry} aria-label="Reload threads" title="Reload threads">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-4 w-4" aria-hidden="true">
              <path d="M21 12a9 9 0 1 1-3-6.7M21 3v6h-6" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </Button>
        </div>

        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted">
          <span aria-live="polite">
            {loading
              ? "Loading"
              : error
                ? "Not loaded"
                : `${threads.length} ${threads.length === 1 ? "thread" : "threads"}${unreadCount > 0 ? `, ${unreadCount} unread` : ""}`}
          </span>
          {label ? (
            <button
              type="button"
              onClick={onClearLabel}
              className="inline-flex min-h-8 items-center gap-1 rounded-full border border-border px-2 font-mono hover:border-danger hover:text-danger"
              aria-label={`Remove the label filter ${label}`}
            >
              {label} <span aria-hidden="true">x</span>
            </button>
          ) : null}
          {archivedCount > 0 ? (
            <button
              type="button"
              onClick={onToggleArchived}
              aria-pressed={showArchived}
              className={`inline-flex min-h-8 items-center rounded-full border px-2 ${
                showArchived ? "border-primary text-fg" : "border-border hover:border-fg/40"
              }`}
            >
              {showArchived ? "Hide archived" : "Show archived"}
            </button>
          ) : null}
        </div>

        {scanning ? (
          <p role="status" className="mt-2 flex items-center gap-2 text-xs text-muted">
            <Spinner className="h-3.5 w-3.5" />
            Reading older mail for this search
          </p>
        ) : null}
      </div>

      {error && !loading ? (
        <div className="p-3">
          <ErrorState error={error} onRetry={onRetry} what="threads" />
        </div>
      ) : loading ? (
        <Skeleton />
      ) : threads.length === 0 ? (
        <div className="p-4">
          <div className="rounded-xl border border-dashed border-border px-4 py-8 text-center">
            <p className="font-medium">
              {searching ? `No thread matches ${query}` : showArchived ? "Nothing archived" : "No threads yet"}
            </p>
            <p className="mt-2 text-sm text-muted">
              {searching
                ? `Search reads the ${scanned} messages loaded so far, then keeps going through older pages.`
                : showArchived
                  ? "Archived threads stay here, with their labels and history, until you open one."
                  : `Every email sent to ${address} becomes a thread you can read here, with its labels and attachments. Your agents read and answer the same messages over the API, so nothing is stranded in a dashboard nobody watches.`}
            </p>
            {searching && hasMore ? (
              <Button className="mt-4" onClick={onLoadMore} busy={loadingMore}>
                Read older mail
              </Button>
            ) : null}
          </div>
        </div>
      ) : (
        <VirtualThreadList
          threads={threads}
          selectedId={selectedId}
          onSelect={onSelect}
          isRead={isRead}
          label="Threads, newest first"
        />
      )}

      {!loading && !error && threads.length > 0 ? (
        <div className="border-t border-border px-3 py-2">
          {hasMore ? (
            <Button className="w-full" onClick={onLoadMore} busy={loadingMore}>
              Load older mail
            </Button>
          ) : (
            <p className="py-1 text-center text-xs text-muted">That is every message in this view.</p>
          )}
          {moreError ? (
            <p role="alert" className="mt-1 text-xs break-words text-danger">
              {errorMessage(moreError)}
            </p>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}
