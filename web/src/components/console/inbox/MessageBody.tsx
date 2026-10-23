"use client";

// The reading pane body: sanitized HTML with remote images blocked until the
// reader asks for them, a plain text fallback, and an honest note about what
// the site policy allows to load.
import { useEffect, useId, useState } from "react";
import type { Message } from "@/lib/console/schemas";
import { bodyText } from "./mail-util";
import { hasVisibleContent, sanitizeEmailHtml, type SanitizedBody } from "./sanitize";

const BODY_CLASS = [
  "text-sm leading-relaxed break-words text-fg",
  "[&_a]:text-primary [&_a]:underline",
  "[&_blockquote]:my-2 [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-3 [&_blockquote]:text-muted",
  "[&_h1]:mt-4 [&_h1]:text-lg [&_h1]:font-semibold [&_h2]:mt-4 [&_h2]:text-base [&_h2]:font-semibold",
  "[&_h3]:mt-3 [&_h3]:text-sm [&_h3]:font-semibold",
  "[&_hr]:my-3 [&_hr]:border-border",
  "[&_img]:my-2 [&_img]:h-auto [&_img]:max-w-full [&_img]:rounded",
  "[&_li]:my-1 [&_li]:ml-4 [&_li]:list-disc",
  "[&_ol_li]:list-decimal",
  "[&_p]:my-2",
  "[&_pre]:my-2 [&_pre]:overflow-x-auto [&_pre]:whitespace-pre-wrap [&_pre]:font-mono [&_pre]:text-xs",
  "[&_table]:my-2 [&_table]:border-collapse [&_table]:text-sm",
  "[&_td]:border [&_td]:border-border [&_td]:p-1.5 [&_th]:border [&_th]:border-border [&_th]:p-1.5 [&_th]:text-left",
  "[&_[data-mai-image]]:my-1 [&_[data-mai-image]]:inline-block [&_[data-mai-image]]:rounded [&_[data-mai-image]]:border",
  "[&_[data-mai-image]]:border-dashed [&_[data-mai-image]]:border-border [&_[data-mai-image]]:px-2",
  "[&_[data-mai-image]]:py-1 [&_[data-mai-image]]:text-xs [&_[data-mai-image]]:text-muted",
].join(" ");

export function MessageBody({ message }: { message: Message }) {
  const [showImages, setShowImages] = useState(false);
  const [body, setBody] = useState<SanitizedBody | null>(null);
  const panelId = useId();

  // Sanitizing needs DOMParser, so it happens after mount; the first paint
  // shows the plain text body, which keeps server and client markup equal.
  useEffect(() => {
    if (!message.html) {
      setBody(null);
      return;
    }
    setBody(sanitizeEmailHtml(message.html, { showImages }));
  }, [message.html, showImages]);

  const html = message.html ?? "";
  const htmlReady = body !== null && hasVisibleContent(body.html);
  const text = bodyText(message);
  const images = body?.blockedImages ?? [];

  return (
    <div className="mt-3">
      {htmlReady ? (
        <div className={BODY_CLASS} dangerouslySetInnerHTML={{ __html: body.html }} />
      ) : text.trim() !== "" ? (
        <pre className="font-sans text-sm leading-relaxed break-words whitespace-pre-wrap">{text}</pre>
      ) : (
        <p className="text-sm text-muted">{message.preview || "This message has no readable body."}</p>
      )}

      {html !== "" ? (
        <div className="mt-3 rounded-lg border border-border bg-bg px-3 py-2">
          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => setShowImages((value) => !value)}
              aria-expanded={showImages}
              aria-controls={panelId}
              className="inline-flex min-h-11 items-center gap-2 rounded-lg border border-border bg-card px-3 text-sm font-medium hover:border-fg/40"
            >
              {showImages ? "Hide images" : "Show images"}
              {images.length > 0 ? (
                <span className="rounded-full bg-bg px-1.5 font-mono text-xs text-muted">{images.length}</span>
              ) : null}
            </button>
            <p className="text-xs text-muted">
              {showImages
                ? "This site loads resources from its own origin only, so remote images may stay blank."
                : "Remote images are blocked, so a sender cannot tell when you opened this mail."}
            </p>
          </div>

          {showImages && images.length > 0 ? (
            <ul id={panelId} className="mt-2 flex flex-col gap-1">
              {images.slice(0, 8).map((image, index) => (
                <li key={`${image.src}-${index}`} className="truncate text-xs">
                  <a
                    href={image.src}
                    target="_blank"
                    rel="noopener noreferrer nofollow"
                    className="text-primary underline"
                    title={image.src}
                  >
                    {image.alt || image.src}
                  </a>
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
