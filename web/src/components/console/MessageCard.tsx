"use client";

import { useState, type FormEvent } from "react";
import { call } from "@/lib/console/client";
import { LabelInput, Message, SYSTEM_LABELS } from "@/lib/console/schemas";
import { useAction } from "@/lib/console/use-resource";
import { useConsole } from "./ConsoleShell";
import { Badge, Button, inputClass, When } from "./kit";


export function screeningBadge(m: Message) {
  if (m.screening === "review") return <Badge tone="warn">held for review</Badge>;
  if (m.screening === "released") return <Badge tone="good">released</Badge>;
  return null;
}

function statusTone(status: Message["status"]): "neutral" | "good" | "bad" {
  if (status === "sent") return "good";
  if (status === "failed") return "bad";
  return "neutral";
}

/**
 * One message with its body, labels (editable with message_update) and, when
 * held for review, a release button (message_release).
 */
export function MessageCard({ message, onChange }: { message: Message; onChange: (m: Message) => void }) {
  const { session, can } = useConsole();
  const action = useAction();
  const [label, setLabel] = useState("");
  const [labelError, setLabelError] = useState<string>();
  const [showHtml, setShowHtml] = useState(false);
  const editable = can("message_update");

  const patchLabels = async (body: { add_labels?: string[]; remove_labels?: string[] }, id: string, success: string) => {
    const out = await action.run(
      id,
      () =>
        call(
          session,
          "PATCH /v0/inboxes/{inbox}/messages/{message}",
          { params: { inbox: message.inbox_id, message: message.id }, body },
          Message,
        ),
      { success, failure: "Could not change the labels" },
    );
    if (out.ok) onChange(out.value);
    return out.ok;
  };

  const addLabel = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const parsed = LabelInput.safeParse(label);
    if (!parsed.success) {
      setLabelError(parsed.error.issues[0]?.message);
      return;
    }
    setLabelError(undefined);
    if (await patchLabels({ add_labels: [parsed.data] }, "add-label", `Label ${parsed.data} added`)) setLabel("");
  };

  const release = async () => {
    const out = await action.run(
      "release",
      () =>
        call(
          session,
          "POST /v0/inboxes/{inbox}/messages/{message}/release",
          { params: { inbox: message.inbox_id, message: message.id } },
          Message,
        ),
      { success: "Message released; message.received was sent to your webhooks", failure: "Could not release the message" },
    );
    if (out.ok) onChange(out.value);
  };

  const body = message.extracted_text ?? message.text ?? null;
  const gk = message.gatekeeper;

  return (
    <article className="rounded-xl border border-border bg-card p-4 sm:p-5">
      <header className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <p className="text-sm break-words">
            <span className="font-medium">{message.from}</span>
            <span className="text-muted"> to {message.to.join(", ") || "nobody"}</span>
          </p>
          {message.cc.length ? <p className="text-xs break-words text-muted">Cc {message.cc.join(", ")}</p> : null}
          <p className="mt-1 font-medium break-words">{message.subject || "(no subject)"}</p>
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2 text-xs text-muted">
          <When iso={message.created_at} />
          <Badge tone={statusTone(message.status)}>{message.direction === "in" ? "inbound" : message.status}</Badge>
          {screeningBadge(message)}
        </div>
      </header>

      {gk && (gk.verdict === "review" || message.screening === "review") ? (
        <div className="mt-3 rounded-lg border border-border border-l-4 border-l-accent bg-bg px-3 py-2 text-sm">
          <p>
            Gatekeeper verdict <strong>{gk.verdict ?? gk.status ?? "unknown"}</strong>
            {gk.score !== undefined ? <> with score {gk.score.toFixed(2)}</> : null}
          </p>
          {gk.reasons.length ? (
            <ul className="mt-1 list-disc pl-5 text-muted">
              {gk.reasons.map((r) => (
                <li key={r} className="break-words">
                  {r}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}

      <div className="mt-4">
        {body !== null && body.trim() !== "" ? (
          <pre className="max-h-[28rem] overflow-auto font-sans text-sm break-words whitespace-pre-wrap">{body}</pre>
        ) : message.html ? (
          <p className="text-sm text-muted">This message has only an HTML body.</p>
        ) : (
          <p className="text-sm text-muted">{message.preview || "No text body."}</p>
        )}
        {message.html ? (
          <div className="mt-3">
            <Button variant="ghost" className="px-2" onClick={() => setShowHtml((v) => !v)}>
              {showHtml ? "Hide HTML" : "Show HTML (scripts and remote content blocked)"}
            </Button>
            {showHtml ? (
              <iframe
                title={`HTML body of ${message.subject || "message"}`}
                sandbox=""
                referrerPolicy="no-referrer"
                srcDoc={`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'">${message.html}`}
                className="mt-2 h-96 w-full rounded-lg border border-border bg-white"
              />
            ) : null}
          </div>
        ) : null}
      </div>

      {message.attachments.length ? (
        <ul className="mt-4 flex flex-wrap gap-2">
          {message.attachments.map((a) => (
            <li key={a.attachment_id}>
              <Badge>
                {a.filename || "attachment"} · {Math.ceil(a.size / 1024)} KB
              </Badge>
            </li>
          ))}
        </ul>
      ) : null}

      <footer className="mt-4 border-t border-border pt-4">
        <div className="flex flex-wrap items-center gap-2">
          {message.labels.length === 0 ? <span className="text-xs text-muted">No labels</span> : null}
          {message.labels.map((l) =>
            editable && !SYSTEM_LABELS.includes(l) ? (
              <button
                key={l}
                type="button"
                onClick={() => patchLabels({ remove_labels: [l] }, `remove:${l}`, `Label ${l} removed`)}
                disabled={action.busy !== null}
                className="inline-flex min-h-8 items-center gap-1 rounded-full border border-border px-2.5 font-mono text-xs hover:border-danger hover:text-danger disabled:opacity-60"
                aria-label={`Remove label ${l}`}
              >
                {l} <span aria-hidden="true">×</span>
              </button>
            ) : (
              <Badge key={l}>{l}</Badge>
            ),
          )}
        </div>
        {editable ? (
          <form onSubmit={addLabel} noValidate className="mt-3 flex flex-col gap-2 sm:flex-row">
            <label htmlFor={`label-${message.id}`} className="sr-only">
              New label
            </label>
            <input
              id={`label-${message.id}`}
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="Add a label"
              autoCapitalize="none"
              spellCheck={false}
              className={`${inputClass} sm:max-w-xs`}
            />
            <Button type="submit" busy={action.busy === "add-label"}>
              Add label
            </Button>
          </form>
        ) : null}
        {labelError ? (
          <p role="alert" className="mt-1 text-sm text-danger">
            {labelError}
          </p>
        ) : null}
        {message.screening === "review" ? (
          <div className="mt-4 flex flex-col gap-2 sm:flex-row sm:items-center">
            <Button variant="primary" busy={action.busy === "release"} onClick={release}>
              Release message
            </Button>
            <p className="text-xs text-muted">
              Needs a key that lists <code className="font-mono">message_release</code>. Full access keys do not hold
              it, even though they report every permission.
            </p>
          </div>
        ) : null}
      </footer>
    </article>
  );
}
