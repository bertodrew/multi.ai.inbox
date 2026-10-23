"use client";

// Attachments with a real download: the API needs the bearer key, so the bytes
// are fetched with the session and handed to the browser as a blob.
import { useState } from "react";
import { useToast } from "@/components/console/Toast";
import { errorMessage } from "@/lib/console/client";
import type { Message } from "@/lib/console/schemas";
import { useConsole } from "@/components/console/ConsoleShell";
import { downloadAttachment } from "./api-extras";
import { formatSize } from "./mail-util";

export function AttachmentList({ message }: { message: Message }) {
  const { session, can } = useConsole();
  const toast = useToast();
  const [busy, setBusy] = useState<string>("");
  const allowed = can("message_read");

  if (message.attachments.length === 0) return null;

  const download = async (attachment: Message["attachments"][number]) => {
    setBusy(attachment.attachment_id);
    try {
      await downloadAttachment(session, message, attachment);
      toast.success("Download started", attachment.filename);
    } catch (error) {
      toast.error("Could not download the attachment", errorMessage(error));
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="mt-3">
      <p className="text-xs font-medium text-muted">
        {message.attachments.length === 1 ? "1 attachment" : `${message.attachments.length} attachments`}
      </p>
      <ul className="mt-1 flex flex-wrap gap-2">
        {message.attachments.map((attachment) => (
          <li key={attachment.attachment_id}>
            <button
              type="button"
              onClick={() => void download(attachment)}
              disabled={!allowed || busy !== ""}
              title={allowed ? `Download ${attachment.filename}` : "Downloading needs the message_read permission"}
              className="inline-flex min-h-11 max-w-full items-center gap-2 rounded-lg border border-border bg-card px-3 py-1.5 text-left text-sm hover:border-fg/40 disabled:cursor-not-allowed disabled:opacity-60"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-4 w-4 shrink-0" aria-hidden="true">
                <path d="M12 3v12m0 0 4-4m-4 4-4-4M4 19h16" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
              <span className="min-w-0">
                <span className="block max-w-[14rem] truncate">{attachment.filename || "attachment"}</span>
                <span className="block text-xs text-muted">
                  {formatSize(attachment.size)}
                  {attachment.content_type ? `, ${attachment.content_type}` : ""}
                  {busy === attachment.attachment_id ? ", downloading" : ""}
                </span>
              </span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
