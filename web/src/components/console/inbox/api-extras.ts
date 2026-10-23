// Two calls the console client does not wrap: label changes applied to every
// message of a thread, and the attachment download, which streams bytes rather
// than JSON. Both go through the same session (base URL + API key) as call().
import { ApiError, call } from "@/lib/console/client";
import { ErrorEnvelope, Message } from "@/lib/console/schemas";
import type { ConsoleSession } from "@/lib/console/session";
import type { Attachment } from "./types";

export type LabelPatch = { add_labels?: string[]; remove_labels?: string[] };

/**
 * Applies one label patch to every message of a thread. Threads are an
 * aggregate the API does not let you relabel in one call, so this walks the
 * messages. Stops at the first failure and reports it.
 */
export async function patchThreadLabels(
  session: ConsoleSession,
  messages: Message[],
  patch: LabelPatch,
): Promise<Message[]> {
  const updated: Message[] = [];
  for (const message of messages) {
    updated.push(
      await call(
        session,
        "PATCH /v0/inboxes/{inbox}/messages/{message}",
        { params: { inbox: message.inbox_id, message: message.id }, body: patch },
        Message,
      ),
    );
  }
  return updated;
}

/**
 * Downloads one attachment with the API key and hands the bytes to the
 * browser. The endpoint is not part of the typed client yet (see api_gaps),
 * so the request is built here from the session base URL.
 */
export async function downloadAttachment(
  session: ConsoleSession,
  message: Message,
  attachment: Attachment,
): Promise<void> {
  const path =
    `/v0/inboxes/${encodeURIComponent(message.inbox_id)}/messages/${encodeURIComponent(message.id)}` +
    `/attachments/${encodeURIComponent(attachment.attachment_id)}`;
  const response = await fetch(session.apiBase + path, {
    headers: { Authorization: `Bearer ${session.apiKey}` },
    credentials: "omit",
    cache: "no-store",
  });
  if (!response.ok) {
    let body: unknown = null;
    try {
      body = await response.json();
    } catch {
      body = null;
    }
    const envelope = ErrorEnvelope.safeParse(body);
    const requestId = response.headers.get("X-Request-Id") ?? "";
    if (envelope.success) {
      throw new ApiError("http", envelope.data.message, response.status, envelope.data.code, requestId);
    }
    throw new ApiError(
      "http",
      `The API answered ${response.status} for the attachment.`.trim(),
      response.status,
      "",
      requestId,
    );
  }
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = attachment.filename || "attachment";
  anchor.rel = "noopener";
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  // Give the browser a moment to start the download before dropping the blob.
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
}
