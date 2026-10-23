// Types local to the webmail. Everything that the API already describes comes
// from schemas.ts; only shapes this page derives (a thread row built from
// message metadata) are defined here, so the shared schemas stay untouched.
import type { Message } from "@/lib/console/schemas";

/** Attachment metadata as it arrives on a message. */
export type Attachment = Message["attachments"][number];

/**
 * One row of the thread list. The API thread list carries no sender, so rows
 * are folded from message metadata: every message knows its thread, sender and
 * labels, which is exactly what a mail list shows.
 */
export type ThreadSummary = {
  thread_id: string;
  inbox_id: string;
  subject: string;
  preview: string;
  /** Raw From header of the newest message. */
  sender: string;
  /** Display name or address, whichever reads better. */
  senderName: string;
  recipients: string[];
  /** ISO timestamp of the newest message in the thread. */
  timestamp: string;
  labels: string[];
  messageCount: number;
  unread: boolean;
  inbound: boolean;
  attachmentCount: number;
  screening: Message["screening"];
  /** Message metadata for the whole conversation, newest first. */
  messages: Message[];
};

export type ComposeMode = "reply" | "forward";
