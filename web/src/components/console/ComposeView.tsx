"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useState, type FormEvent } from "react";
import { call } from "@/lib/console/client";
import { InboxList, SendInput, SendResult } from "@/lib/console/schemas";
import { useAction, useResource } from "@/lib/console/use-resource";
import { useConsole } from "./ConsoleShell";
import { Button, Card, EmptyState, ErrorState, Field, inputClass, LoadingState, PageTitle } from "./kit";
import { PermissionNote } from "./PermissionNote";

type Errors = Partial<Record<"from" | "to" | "cc" | "bcc" | "subject" | "text" | "form", string>>;

export function ComposeView() {
  const { session, can } = useConsole();
  const params = useSearchParams();
  const action = useAction();
  const inboxes = useResource(
    "compose-inboxes",
    (signal) => call(session, "GET /v0/inboxes", { params: {}, query: { limit: 100 }, signal }, InboxList),
    "inboxes",
  );
  const [from, setFrom] = useState(params.get("from") ?? "");
  const [to, setTo] = useState("");
  const [cc, setCc] = useState("");
  const [bcc, setBcc] = useState("");
  const [subject, setSubject] = useState("");
  const [text, setText] = useState("");
  const [errors, setErrors] = useState<Errors>({});
  const [sent, setSent] = useState<{ inbox: string; thread: string } | null>(null);
  const [idempotencyKey, setIdempotencyKey] = useState(() => crypto.randomUUID());

  if (!can("message_send")) return <PermissionNote permission="message_send" action="send mail" />;

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const next: Errors = {};
    const sender = from || inboxes.data?.inboxes[0]?.inbox_id || "";
    if (!sender) next.from = "Pick the inbox to send from";
    const parsed = SendInput.safeParse({ to, cc, bcc, subject, text });
    if (!parsed.success) {
      for (const issue of parsed.error.issues) {
        const field = issue.path[0];
        const key = field === "to" || field === "cc" || field === "bcc" || field === "subject" || field === "text" ? field : "form";
        next[key] ??= issue.message;
      }
    }
    setErrors(next);
    if (!parsed.success || !sender) return;
    const out = await action.run(
      "send",
      () =>
        call(
          session,
          "POST /v0/inboxes/{inbox}/messages/send",
          { params: { inbox: sender }, body: parsed.data, idempotencyKey },
          SendResult,
        ),
      { success: "Message queued for sending", failure: "Could not send the message" },
    );
    if (out.ok) {
      setSent({ inbox: sender, thread: out.value.thread_id });
      setTo("");
      setCc("");
      setBcc("");
      setSubject("");
      setText("");
      setIdempotencyKey(crypto.randomUUID());
    }
  };

  return (
    <>
      <PageTitle title="Compose" intro="Send a new message; it starts a new thread. Replies are written from the thread." />
      {sent ? (
        <div role="status" className="mb-4 rounded-xl border border-border border-l-4 border-l-success bg-card px-4 py-3 text-sm">
          Queued.{" "}
          <Link
            href={`/console/inbox/?address=${encodeURIComponent(sent.inbox)}&thread=${encodeURIComponent(sent.thread)}`}
            className="text-primary underline"
          >
            Open the thread
          </Link>
        </div>
      ) : null}
      {inboxes.data === undefined ? (
        inboxes.error ? (
          <ErrorState error={inboxes.error} onRetry={inboxes.reload} what="inboxes" />
        ) : (
          <LoadingState label="Loading inboxes" />
        )
      ) : inboxes.data.inboxes.length === 0 ? (
        <EmptyState title="No inbox to send from">
          <Link href="/console/" className="text-primary underline">
            Create an inbox
          </Link>{" "}
          first.
        </EmptyState>
      ) : (
        <Card>
          <form onSubmit={submit} noValidate className="flex flex-col gap-4">
            <Field label="From" htmlFor="compose-from" error={errors.from}>
              <select
                id="compose-from"
                value={from || inboxes.data.inboxes[0]?.inbox_id}
                onChange={(e) => setFrom(e.target.value)}
                className={inputClass}
              >
                {from && !inboxes.data.inboxes.some((i) => i.inbox_id === from) ? <option value={from}>{from}</option> : null}
                {inboxes.data.inboxes.map((i) => (
                  <option key={i.id} value={i.inbox_id}>
                    {i.display_name ? `${i.display_name} <${i.inbox_id}>` : i.inbox_id}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="To" htmlFor="compose-to" error={errors.to} hint="Separate addresses with commas.">
              <input id="compose-to" type="text" inputMode="email" autoCapitalize="none" spellCheck={false} value={to} onChange={(e) => setTo(e.target.value)} className={inputClass} />
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Cc" htmlFor="compose-cc" error={errors.cc}>
                <input id="compose-cc" type="text" inputMode="email" autoCapitalize="none" spellCheck={false} value={cc} onChange={(e) => setCc(e.target.value)} className={inputClass} />
              </Field>
              <Field label="Bcc" htmlFor="compose-bcc" error={errors.bcc}>
                <input id="compose-bcc" type="text" inputMode="email" autoCapitalize="none" spellCheck={false} value={bcc} onChange={(e) => setBcc(e.target.value)} className={inputClass} />
              </Field>
            </div>
            <Field label="Subject" htmlFor="compose-subject" error={errors.subject}>
              <input id="compose-subject" value={subject} onChange={(e) => setSubject(e.target.value)} className={inputClass} />
            </Field>
            <Field label="Message" htmlFor="compose-text" error={errors.text ?? errors.form}>
              <textarea id="compose-text" rows={10} value={text} onChange={(e) => setText(e.target.value)} className={inputClass} />
            </Field>
            <div>
              <Button type="submit" variant="primary" busy={action.busy === "send"}>
                Send
              </Button>
            </div>
          </form>
        </Card>
      )}
    </>
  );
}
