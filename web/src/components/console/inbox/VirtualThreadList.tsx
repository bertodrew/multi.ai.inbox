"use client";

// A windowed thread list: only the rows in view (plus a small overscan) are
// mounted, so a mailbox with thousands of threads scrolls at a steady cost.
// Rows are a fixed height, which is what makes the maths exact.
import { useCallback, useEffect, useRef, useState } from "react";
import { initials, threadTime } from "./mail-util";
import type { ThreadSummary } from "./types";

const ROW_HEIGHT = 112;
const OVERSCAN = 6;

function Paperclip() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-3.5 w-3.5" aria-hidden="true">
      <path
        d="M21 12.5 12.5 21a5 5 0 0 1-7-7l8-8a3.5 3.5 0 0 1 5 5l-8 8a2 2 0 0 1-3-3l7-7"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

export function ThreadRow({
  thread,
  selected,
  read,
  onSelect,
}: {
  thread: ThreadSummary;
  selected: boolean;
  read: boolean;
  onSelect: (thread: ThreadSummary) => void;
}) {
  const bold = !read;
  return (
    <button
      type="button"
      id={`thread-${thread.thread_id}`}
      role="option"
      aria-selected={selected}
      onClick={() => onSelect(thread)}
      className={`flex h-full w-full items-stretch gap-2.5 px-3 py-2.5 text-left transition-colors ${
        selected ? "bg-bg" : "hover:bg-bg"
      }`}
    >
      <span
        className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-bg font-mono text-xs text-muted"
        aria-hidden="true"
      >
        {initials(thread.senderName)}
      </span>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="flex items-baseline gap-2">
          <span className={`min-w-0 flex-1 truncate text-sm ${bold ? "font-semibold" : "font-medium"}`}>
            {thread.senderName}
          </span>
          <span className="shrink-0 text-[11px] text-muted">
            <time dateTime={thread.timestamp}>{threadTime(thread.timestamp)}</time>
          </span>
        </span>
        <span className="mt-0.5 flex items-center gap-1.5">
          {thread.unread && !read ? (
            <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-primary" aria-hidden="true" />
          ) : null}
          <span className={`min-w-0 flex-1 truncate text-sm ${bold ? "font-medium" : "text-fg/90"}`}>
            {thread.subject || "(no subject)"}
          </span>
        </span>
        <span className="mt-0.5 line-clamp-2 min-h-[2rem] text-xs text-muted">{thread.preview || "No preview"}</span>
        <span className="mt-auto flex items-center gap-1.5 overflow-hidden pt-1 text-[11px] text-muted">
          {thread.messageCount > 1 ? (
            <span className="rounded-full border border-border px-1.5 font-mono">{thread.messageCount}</span>
          ) : null}
          {thread.attachmentCount > 0 ? (
            <span className="inline-flex items-center gap-1 rounded-full border border-border px-1.5">
              <Paperclip />
              {thread.attachmentCount}
            </span>
          ) : null}
          {thread.labels
            .filter((label) => label !== "received" && label !== "sent" && label !== "unread")
            .slice(0, 3)
            .map((label) => (
              <span key={label} className="max-w-[7rem] truncate rounded-full bg-bg px-1.5 font-mono">
                {label}
              </span>
            ))}
          {!read && thread.unread ? <span className="sr-only">Unread</span> : null}
        </span>
      </span>
    </button>
  );
}

export function VirtualThreadList({
  threads,
  selectedId,
  onSelect,
  isRead,
  label,
}: {
  threads: ThreadSummary[];
  selectedId: string;
  onSelect: (thread: ThreadSummary) => void;
  isRead: (thread: ThreadSummary) => boolean;
  label: string;
}) {
  const scroller = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewport, setViewport] = useState(560);

  useEffect(() => {
    const element = scroller.current;
    if (!element) return;
    const measure = () => setViewport(element.clientHeight || 560);
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const onScroll = useCallback(() => {
    const element = scroller.current;
    if (element) setScrollTop(element.scrollTop);
  }, []);

  const total = threads.length * ROW_HEIGHT;
  const first = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
  const last = Math.min(threads.length, Math.ceil((scrollTop + viewport) / ROW_HEIGHT) + OVERSCAN);
  const visible = threads.slice(first, last);

  // Keep the open thread on screen when the selection moves by keyboard.
  useEffect(() => {
    const element = scroller.current;
    if (!element || !selectedId) return;
    const index = threads.findIndex((thread) => thread.thread_id === selectedId);
    if (index < 0) return;
    const top = index * ROW_HEIGHT;
    const bottom = top + ROW_HEIGHT;
    if (top < element.scrollTop) element.scrollTop = top;
    else if (bottom > element.scrollTop + element.clientHeight) element.scrollTop = bottom - element.clientHeight;
  }, [selectedId, threads]);

  const move = (delta: number) => {
    if (threads.length === 0) return;
    const index = threads.findIndex((thread) => thread.thread_id === selectedId);
    const next = Math.min(threads.length - 1, Math.max(0, index < 0 ? 0 : index + delta));
    const thread = threads[next];
    if (thread) onSelect(thread);
  };

  return (
    <div
      ref={scroller}
      onScroll={onScroll}
      onKeyDown={(event) => {
        if (event.key === "ArrowDown") {
          event.preventDefault();
          move(1);
        } else if (event.key === "ArrowUp") {
          event.preventDefault();
          move(-1);
        } else if (event.key === "Home") {
          event.preventDefault();
          const thread = threads[0];
          if (thread) onSelect(thread);
        } else if (event.key === "End") {
          event.preventDefault();
          const thread = threads.at(-1);
          if (thread) onSelect(thread);
        }
      }}
      tabIndex={0}
      role="listbox"
      aria-label={label}
      aria-activedescendant={selectedId ? `thread-${selectedId}` : undefined}
      className="relative min-h-0 flex-1 overflow-y-auto overscroll-contain outline-offset-[-2px]"
    >
      <div style={{ height: total }} className="relative w-full">
        {visible.map((thread, offset) => (
          <div
            key={thread.thread_id}
            className="absolute inset-x-0 border-b border-border"
            style={{ top: (first + offset) * ROW_HEIGHT, height: ROW_HEIGHT }}
          >
            <ThreadRow
              thread={thread}
              selected={thread.thread_id === selectedId}
              read={isRead(thread)}
              onSelect={onSelect}
            />
          </div>
        ))}
      </div>
    </div>
  );
}
