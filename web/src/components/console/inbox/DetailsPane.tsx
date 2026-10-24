"use client";

// Column 3: who is on the thread, when it started, and the labels. Label edits
// walk the messages because the API patches one message at a time.
import { useState, type FormEvent } from "react";
import { useToast } from "@/components/console/Toast";
import { useAction } from "@/lib/console/use-resource";
import { errorMessage } from "@/lib/console/client";
import { LabelInput, type Message } from "@/lib/console/schemas";
import { useConsole } from "@/components/console/ConsoleShell";
import { Badge, Button, inputClass } from "@/components/console/kit";
import { patchThreadLabels } from "./api-extras";
import { customLabels, fullTime, isHeld, parseAddress } from "./mail-util";
import type { ThreadSummary } from "./types";

function Party({ label, value }: { label: string; value: string }) {
  const { name, address } = parseAddress(value);
  return (
    <li className="text-xs">
      <span className="text-muted">{label} </span>
      <span className="break-words">{name || address}</span>
      {name && address ? <span className="block truncate font-mono text-[11px] text-muted">{address}</span> : null}
    </li>
  );
}

function authValue(value: unknown): string {
  if (value === null || value === undefined) return "none";
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return String(value);
  try {
    return JSON.stringify(value);
  } catch {
    return "unreadable";
  }
}

export function DetailsPane({
  address,
  summary,
  archived,
  unread,
  onChanged,
  onMarkUnread,
  onUnarchive,
  idPrefix = "thread",
}: {
  address: string;
  summary: ThreadSummary | undefined;
  archived: boolean;
  unread: boolean;
  onChanged: () => void;
  onMarkUnread: () => void;
  onUnarchive: () => void;
  idPrefix?: string;
}) {
  const { session, can } = useConsole();
  const toast = useToast();
  const action = useAction();
  const [label, setLabel] = useState("");
  const [labelError, setLabelError] = useState<string>();
  const editable = can("message_update");
  const messages: Message[] = summary?.messages ?? [];
  // The thread endpoint answers oldest first and the list newest first, so
  // order the two ends by timestamp instead of trusting the array.
  const newest = messages.reduce<Message | undefined>(
    (best, message) => (!best || new Date(message.created_at) > new Date(best.created_at) ? message : best),
    undefined,
  );
  const oldest = messages.reduce<Message | undefined>(
    (best, message) => (!best || new Date(message.created_at) < new Date(best.created_at) ? message : best),
    undefined,
  );
  const labels = summary?.labels ?? [];
  const removable = customLabels(labels);
  const participants = [...new Set(messages.flatMap((message) => [message.from, ...message.to, ...message.cc]))].filter(Boolean);

  // The workspace renders this pane twice, in the third column on a desktop and
  // under the reader on a phone. Two landmarks with the same name are noise for
  // a screen reader, so the small screen copy says so.
  const paneLabel = idPrefix.startsWith("thread-mobile") ? "Thread details, small screen" : "Thread details";

  if (!summary) {
    return (
      <aside
        aria-label={paneLabel}
        className="flex min-h-0 min-w-0 flex-col gap-3 overflow-y-auto bg-card px-3 py-3"
      >
        <p className="font-medium">No thread open</p>
        <p className="text-sm text-muted">
          Pick a thread to see the people on it, its labels and its authentication results. A label you add here is
          written to the API, so the agent that reads this inbox sees it too.
        </p>
      </aside>
    );
  }

  const patch = async (body: { add_labels?: string[]; remove_labels?: string[] }, done: string) => {
    if (!editable || messages.length === 0) return false;
    try {
      await patchThreadLabels(session, messages, body);
      toast.success(done, messages.length === 1 ? undefined : `Applied to ${messages.length} messages`);
      onChanged();
      return true;
    } catch (error) {
      toast.error("Could not change the labels", errorMessage(error));
      return false;
    }
  };

  const addLabel = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const parsed = LabelInput.safeParse(label);
    if (!parsed.success) {
      setLabelError(parsed.error.issues[0]?.message);
      return;
    }
    setLabelError(undefined);
    if (await patch({ add_labels: [parsed.data] }, `Label ${parsed.data} added`)) setLabel("");
  };

  return (
    <aside aria-label={paneLabel} className="flex min-h-0 min-w-0 flex-col gap-4 overflow-y-auto bg-card px-3 py-3">
      <section>
        <h2 className="text-xs font-semibold tracking-wide text-muted uppercase">Details</h2>
        <dl className="mt-2 flex flex-col gap-1.5 text-xs">
          <div className="flex justify-between gap-3">
            <dt className="text-muted">Inbox</dt>
            <dd className="truncate font-mono">{address}</dd>
          </div>
          <div className="flex justify-between gap-3">
            <dt className="text-muted">Messages</dt>
            <dd className="font-mono">{summary?.messageCount ?? messages.length}</dd>
          </div>
          {newest ? (
            <div className="flex justify-between gap-3">
              <dt className="text-muted">Newest</dt>
              <dd className="text-right">{fullTime(newest.created_at)}</dd>
            </div>
          ) : null}
          {oldest ? (
            <div className="flex justify-between gap-3">
              <dt className="text-muted">Started</dt>
              <dd className="text-right">{fullTime(oldest.created_at)}</dd>
            </div>
          ) : null}
          {newest ? (
            <div className="flex justify-between gap-3">
              <dt className="text-muted">Size</dt>
              <dd className="font-mono">{newest.attachments.length > 0 ? `${newest.attachments.length} attached` : "no attachments"}</dd>
            </div>
          ) : null}
        </dl>
      </section>

      <section>
        <h2 className="text-xs font-semibold tracking-wide text-muted uppercase">People</h2>
        {participants.length === 0 ? (
          <p className="mt-2 text-xs text-muted">Open the thread to see who is on it.</p>
        ) : (
          <ul className="mt-2 flex flex-col gap-1.5">
            {participants.slice(0, 8).map((party, index) => (
              <Party key={`${party}-${index}`} label={index === 0 ? "From" : "With"} value={party} />
            ))}
          </ul>
        )}
      </section>

      <section>
        <div className="flex items-center justify-between gap-2">
          <h2 className="text-xs font-semibold tracking-wide text-muted uppercase">Labels</h2>
          {archived ? (
            <button
              type="button"
              onClick={onUnarchive}
              className="inline-flex min-h-8 items-center rounded-full border border-border px-2 text-[11px] hover:border-fg/40"
            >
              Unarchive
            </button>
          ) : null}
        </div>
        <div className="mt-2 flex flex-wrap gap-1.5">
          {labels.length === 0 ? <span className="text-xs text-muted">No labels</span> : null}
          {labels.map((value) =>
            editable && removable.includes(value) ? (
              <button
                key={value}
                type="button"
                onClick={() => void patch({ remove_labels: [value] }, `Label ${value} removed`)}
                disabled={action.busy !== null}
                className="inline-flex min-h-8 items-center gap-1 rounded-full border border-border px-2 font-mono text-[11px] hover:border-danger hover:text-danger disabled:opacity-60"
                aria-label={`Remove label ${value}`}
              >
                {value} <span aria-hidden="true">x</span>
              </button>
            ) : (
              <Badge key={value}>{value}</Badge>
            ),
          )}
        </div>

        {editable ? (
          <form onSubmit={addLabel} noValidate className="mt-2">
            <label htmlFor={`${idPrefix}-label`} className="sr-only">
              Add a label to this thread
            </label>
            <div className="flex gap-2">
              <input
                id={`${idPrefix}-label`}
                value={label}
                onChange={(event) => setLabel(event.target.value)}
                placeholder="Add a label"
                autoCapitalize="none"
                spellCheck={false}
                className={`${inputClass} min-w-0 flex-1`}
              />
              <Button type="submit" busy={action.busy === "add-label"} disabled={messages.length === 0}>
                Add
              </Button>
            </div>
            {labelError ? (
              <p role="alert" className="mt-1 text-xs text-danger">
                {labelError}
              </p>
            ) : null}
          </form>
        ) : (
          <p className="mt-2 text-xs text-muted">Changing labels needs a key with the message_update permission.</p>
        )}
      </section>

      <section>
        <h2 className="text-xs font-semibold tracking-wide text-muted uppercase">Read state</h2>
        <div className="mt-2">
          {unread ? (
            <p className="text-xs text-muted">This thread is unread in this browser.</p>
          ) : (
            <Button className="w-full" onClick={onMarkUnread} disabled={!editable}>
              Mark as unread
            </Button>
          )}
          {!editable && !unread ? (
            <p className="mt-1 text-xs text-muted">Unread state is stored as a message label, so it needs message_update.</p>
          ) : null}
        </div>
      </section>

      {messages.some(isHeld) ? (
        <section className="rounded-lg border border-border border-l-4 border-l-accent bg-bg px-2.5 py-2">
          <h2 className="text-xs font-semibold">Held for review</h2>
          <p className="mt-1 text-xs text-muted">
            The gatekeeper held part of this thread. Release it in the reading pane to notify your webhooks.
          </p>
        </section>
      ) : null}

      {newest?.authentication_results && Object.keys(newest.authentication_results).length > 0 ? (
        <section>
          <h2 className="text-xs font-semibold tracking-wide text-muted uppercase">Authentication</h2>
          <ul className="mt-2 flex flex-col gap-1">
            {Object.entries(newest.authentication_results)
              .slice(0, 8)
              .map(([key, value]) => (
                <li key={key} className="flex items-baseline justify-between gap-2 text-[11px]">
                  <span className="font-mono text-muted">{key}</span>
                  <span className="max-w-[9rem] truncate text-right" title={authValue(value)}>
                    {authValue(value)}
                  </span>
                </li>
              ))}
          </ul>
        </section>
      ) : null}
    </aside>
  );
}
