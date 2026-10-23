// Pure helpers for the webmail: address parsing, dates, sizes, folding message
// metadata into thread rows and the search predicate. No React, no I/O.
import type { Message } from "@/lib/console/schemas";
import type { ThreadSummary } from "./types";

const UNREAD = "unread";

/** Splits "Ada Lovelace <ada@example.com>" into its two parts. */
export function parseAddress(raw: string): { name: string; address: string } {
  const value = (raw ?? "").trim();
  const open = value.lastIndexOf("<");
  const close = value.lastIndexOf(">");
  if (open > -1 && close > open) {
    const name = value.slice(0, open).trim().replace(/^"|"$/g, "");
    return { name, address: value.slice(open + 1, close).trim() };
  }
  return { name: "", address: value };
}

/** A short, human label for a sender: the display name, else the address. */
export function senderName(raw: string): string {
  const { name, address } = parseAddress(raw);
  return name || address || "Unknown sender";
}

/** One or two letters for the round avatar in the list. */
export function initials(name: string): string {
  const letters = name
    .replace(/[^\p{L}\p{N} ]/gu, " ")
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0] ?? "")
    .join("");
  return (letters || "?").toUpperCase();
}

const timeOnly = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
const monthDay = new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" });
const withYear = new Intl.DateTimeFormat(undefined, { year: "numeric", month: "short", day: "numeric" });
const full = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });

/**
 * Compact list timestamp: the time for today, the month and day for this year,
 * the year before that. Gmail style, so a column of rows scans quickly.
 */
export function threadTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  const now = new Date();
  if (date.toDateString() === now.toDateString()) return timeOnly.format(date);
  if (date.getFullYear() === now.getFullYear()) return monthDay.format(date);
  return withYear.format(date);
}

export function fullTime(iso: string): string {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : full.format(date);
}

export function formatSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "0 KB";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** Labels the platform writes itself; they are shown but never edited. */
const PLATFORM_LABELS = new Set(["received", "sent", "queued", "failed", "bounced", "unread"]);

export function customLabels(labels: string[]): string[] {
  return labels.filter((label) => !PLATFORM_LABELS.has(label) && label !== "archived");
}

export function isHeld(message: Message): boolean {
  return message.screening === "review" || message.gatekeeper?.verdict === "review";
}

/** Drops duplicates by message id, keeping the first (newest) occurrence. */
export function mergeMessages(current: Message[], incoming: Message[]): Message[] {
  const seen = new Set(current.map((message) => message.id));
  const merged = [...current];
  for (const message of incoming) {
    if (seen.has(message.id)) continue;
    seen.add(message.id);
    merged.push(message);
  }
  return merged;
}

/**
 * Folds message metadata (newest first) into one row per thread. A thread's
 * sender, timestamp and preview come from its newest message; the unread flag
 * and attachment count come from the whole conversation.
 */
export function groupThreads(messages: Message[]): ThreadSummary[] {
  const byThread = new Map<string, ThreadSummary>();
  for (const message of messages) {
    const existing = byThread.get(message.thread_id);
    if (!existing) {
      byThread.set(message.thread_id, {
        thread_id: message.thread_id,
        inbox_id: message.inbox_id,
        subject: message.subject,
        preview: message.preview,
        sender: message.from,
        senderName: senderName(message.from),
        recipients: [...message.to, ...message.cc],
        timestamp: message.created_at,
        labels: [...message.labels],
        messageCount: 1,
        unread: message.labels.includes(UNREAD),
        inbound: message.direction === "in",
        attachmentCount: message.attachments.length,
        screening: message.screening,
        messages: [message],
      });
      continue;
    }
    existing.messageCount += 1;
    existing.messages.push(message);
    if (message.labels.includes(UNREAD)) existing.unread = true;
    existing.attachmentCount += message.attachments.length;
    if (message.screening === "review") existing.screening = "review";
    for (const label of message.labels) {
      if (!existing.labels.includes(label)) existing.labels.push(label);
    }
  }
  return [...byThread.values()];
}

/** Everything a search may look at, lowercased once per row. */
export function searchHaystack(thread: ThreadSummary): string {
  const parts = [
    thread.sender,
    thread.senderName,
    thread.recipients.join(" "),
    thread.subject,
    thread.preview,
    thread.labels.join(" "),
    ...thread.messages.map((message) => message.preview),
  ];
  return parts.join(" ").toLowerCase();
}

/** Every token must appear somewhere in the row: "ada invoice" narrows down. */
export function matchesQuery(thread: ThreadSummary, tokens: string[]): boolean {
  if (tokens.length === 0) return true;
  const hay = searchHaystack(thread);
  return tokens.every((token) => hay.includes(token));
}

export function tokensOf(query: string): string[] {
  return query.toLowerCase().split(/\s+/).filter(Boolean);
}

/** Body text used for quoting: the readable text, HTML stripped as a fallback. */
export function bodyText(message: Message): string {
  const text = message.extracted_text ?? message.text;
  if (text && text.trim() !== "") return text;
  if (message.html) {
    return message.html
      .replace(/<style[\s\S]*?<\/style>/gi, " ")
      .replace(/<script[\s\S]*?<\/script>/gi, " ")
      .replace(/<br\s*\/?>/gi, "\n")
      .replace(/<\/(p|div|li|tr|h[1-6])>/gi, "\n")
      .replace(/<[^>]+>/g, " ")
      .replace(/&nbsp;/g, " ")
      .replace(/&amp;/g, "&")
      .replace(/&lt;/g, "<")
      .replace(/&gt;/g, ">")
      .replace(/[ \t]+/g, " ")
      .replace(/\n{3,}/g, "\n\n")
      .trim();
  }
  return message.preview ?? "";
}

/** The quoted block a forward starts with. */
export function quotedMessage(message: Message): string {
  const header = [
    "Forwarded message",
    `From: ${message.from}`,
    `Date: ${fullTime(message.created_at)}`,
    `Subject: ${message.subject || "(no subject)"}`,
    `To: ${message.to.join(", ")}`,
  ];
  if (message.cc.length) header.push(`Cc: ${message.cc.join(", ")}`);
  return `\n\n${header.join("\n")}\n\n${bodyText(message)}`;
}

/** "Fwd:" prefixes once, the way every mail client does it. */
export function forwardSubject(subject: string): string {
  const trimmed = subject.trim();
  if (/^fwd:/i.test(trimmed) || /^fw:/i.test(trimmed)) return trimmed;
  return trimmed ? `Fwd: ${trimmed}` : "Fwd: (no subject)";
}
