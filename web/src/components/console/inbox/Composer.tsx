"use client";

// Reply and forward forms. Both queue mail through the API and keep one
// idempotency key per draft, so a retry after a network error cannot send twice.
import { useState, type FormEvent } from "react";
import { useAction } from "@/lib/console/use-resource";
import { call } from "@/lib/console/client";
import { ReplyInput, SendInput, SendResult, type Message } from "@/lib/console/schemas";
import { useConsole } from "@/components/console/ConsoleShell";
import { Button, Field, inputClass } from "@/components/console/kit";
import { forwardSubject, quotedMessage } from "./mail-util";
import type { ComposeMode } from "./types";

export function Composer({
  mode,
  message,
  onClose,
  onSent,
}: {
  mode: ComposeMode;
  message: Message;
  onClose: () => void;
  onSent: () => void;
}) {
  const { session, can } = useConsole();
  const action = useAction();
  const [text, setText] = useState(mode === "forward" ? quotedMessage(message) : "");
  const [to, setTo] = useState("");
  const [replyAll, setReplyAll] = useState(false);
  const [error, setError] = useState<string>();
  const [idempotencyKey, setIdempotencyKey] = useState(() => crypto.randomUUID());
  const allowed = can("message_send");
  const subject = mode === "forward" ? forwardSubject(message.subject) : message.subject;

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (mode === "reply") {
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
            { params: { inbox: message.inbox_id, message: message.id }, body: parsed.data, idempotencyKey },
            SendResult,
          ),
        { success: "Reply queued for sending", failure: "Could not send the reply" },
      );
      if (out.ok) {
        setText("");
        setIdempotencyKey(crypto.randomUUID());
        onSent();
        onClose();
      }
      return;
    }

    const parsed = SendInput.safeParse({ to, cc: "", bcc: "", subject, text });
    if (!parsed.success) {
      setError(parsed.error.issues[0]?.message);
      return;
    }
    setError(undefined);
    const outcome = await action.run(
      "forward",
      () =>
        call(
          session,
          "POST /v0/inboxes/{inbox}/messages/send",
          { params: { inbox: message.inbox_id }, body: parsed.data, idempotencyKey },
          SendResult,
        ),
      { success: "Forward queued for sending", failure: "Could not forward the message" },
    );
    if (outcome.ok) {
      setIdempotencyKey(crypto.randomUUID());
      onSent();
      onClose();
    }
  };

  return (
    <form onSubmit={submit} noValidate className="mt-4 rounded-xl border border-border bg-card p-3 sm:p-4">
      <div className="flex items-start justify-between gap-2">
        <h3 className="text-sm font-semibold">{mode === "reply" ? `Reply to ${message.from}` : "Forward this message"}</h3>
        <button
          type="button"
          onClick={onClose}
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-muted hover:text-fg"
          aria-label="Close the composer"
        >
          <span aria-hidden="true" className="text-lg leading-none">
            x
          </span>
        </button>
      </div>

      {!allowed ? (
        <p role="note" className="mt-2 text-sm text-muted">
          Sending needs an API key with the message_send permission. Ask an owner for one, or reply from the API.
        </p>
      ) : null}

      {mode === "forward" ? (
        <div className="mt-3 flex flex-col gap-3">
          <Field label="To" htmlFor="forward-to" hint="One address, or several separated by commas.">
            <input
              id="forward-to"
              value={to}
              onChange={(event) => setTo(event.target.value)}
              placeholder="name@example.com"
              autoComplete="off"
              inputMode="email"
              spellCheck={false}
              className={inputClass}
            />
          </Field>
          <p className="text-xs text-muted">
            Subject <span className="font-mono text-fg">{subject}</span>
          </p>
        </div>
      ) : null}

      <div className="mt-3">
        <Field
          label={mode === "reply" ? "Your reply" : "Your message"}
          htmlFor="composer-text"
          error={error}
          hint={mode === "reply" ? "The reply stays in this thread and keeps its history." : "The forwarded mail is quoted below your text."}
        >
          <textarea
            id="composer-text"
            rows={mode === "forward" ? 8 : 5}
            value={text}
            onChange={(event) => setText(event.target.value)}
            className={inputClass}
          />
        </Field>
      </div>

      <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        {mode === "reply" ? (
          <label className="flex min-h-11 items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={replyAll}
              onChange={(event) => setReplyAll(event.target.checked)}
              className="h-4 w-4"
            />
            Reply to every recipient
          </label>
        ) : (
          <span className="text-xs text-muted">Queued mail can be retried safely: the same draft never sends twice.</span>
        )}
        <div className="flex gap-2">
          <Button onClick={onClose}>Cancel</Button>
          <Button
            type="submit"
            variant="primary"
            busy={action.busy === "reply" || action.busy === "forward"}
            disabled={!allowed}
          >
            {mode === "reply" ? "Send reply" : "Send forward"}
          </Button>
        </div>
      </div>
      {action.busy === null && error ? (
        <p role="alert" className="mt-2 text-sm text-danger">
          {error}
        </p>
      ) : null}
    </form>
  );
}
