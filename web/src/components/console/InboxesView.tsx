"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";
import { call, callNoContent } from "@/lib/console/client";
import { CreateInboxInput, Inbox, InboxList } from "@/lib/console/schemas";
import { useAction, useResource } from "@/lib/console/use-resource";
import { useConsole } from "./ConsoleShell";
import { Badge, Button, Card, ConfirmButton, EmptyState, ErrorState, Field, inputClass, LoadingState, PageTitle, When } from "./kit";
import { PermissionNote } from "./PermissionNote";
import { PushSettings } from "./PushSettings";

type Page = { inboxes: Inbox[]; next?: string };

export function InboxesView() {
  const { session, can } = useConsole();
  const action = useAction();
  const list = useResource<Page>(
    "inboxes",
    async (signal) => {
      const res = await call(session, "GET /v0/inboxes", { params: {}, query: { limit: 50 }, signal }, InboxList);
      return { inboxes: res.inboxes, next: res.next_page_token };
    },
    "inboxes",
  );

  const loadMore = async (token: string) => {
    const out = await action.run(
      "more",
      () => call(session, "GET /v0/inboxes", { params: {}, query: { limit: 50, page_token: token } }, InboxList),
      { failure: "Could not load more inboxes" },
    );
    if (out.ok) {
      list.mutate((cur) => ({ inboxes: [...cur.inboxes, ...out.value.inboxes], next: out.value.next_page_token }));
    }
  };

  const toggleIntake = async (inbox: Inbox) => {
    const out = await action.run(
      `intake:${inbox.inbox_id}`,
      () =>
        call(session, "PATCH /v0/inboxes/{inbox}", {
          params: { inbox: inbox.inbox_id },
          body: { ticket_intake: !inbox.ticket_intake },
        }, Inbox),
      {
        success: inbox.ticket_intake ? "Support intake off" : "Support intake on",
        failure: `Could not change ${inbox.inbox_id}`,
      },
    );
    if (out.ok) list.reload();
  };

  const remove = async (inbox: Inbox): Promise<boolean> => {
    const out = await action.run(
      `delete:${inbox.inbox_id}`,
      () => callNoContent(session, "DELETE /v0/inboxes/{inbox}", { params: { inbox: inbox.inbox_id } }),
      { success: `Deleted ${inbox.inbox_id}`, failure: `Could not delete ${inbox.inbox_id}` },
    );
    if (out.ok) list.mutate((cur) => ({ ...cur, inboxes: cur.inboxes.filter((i) => i.id !== inbox.id) }));
    return out.ok;
  };

  return (
    <>
      <PageTitle
        title="Inboxes"
        intro="Every address your agents read and send from. Open one to see its threads."
        actions={
          <Button onClick={list.reload} busy={list.loading && list.data !== undefined}>
            Refresh
          </Button>
        }
      />
      {can("inbox_write") ? (
        <CreateInbox onCreated={(inbox) => list.mutate((cur) => ({ ...cur, inboxes: [inbox, ...cur.inboxes.filter((i) => i.id !== inbox.id)] }))} />
      ) : (
        <PermissionNote permission="inbox_write" action="create or delete inboxes" />
      )}

      <div className="mt-6">
        {list.data === undefined ? (
          list.error ? (
            <ErrorState error={list.error} onRetry={list.reload} what="inboxes" />
          ) : (
            <LoadingState label="Loading inboxes" />
          )
        ) : list.data.inboxes.length === 0 ? (
          <EmptyState title="No inboxes yet">Create the first one above; leave the name empty for a random address.</EmptyState>
        ) : (
          <>
            <ul className="divide-y divide-border rounded-xl border border-border bg-card">
              {list.data.inboxes.map((inbox) => (
                <li key={inbox.id} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
                  <div className="min-w-0">
                    <Link
                      href={`/console/inbox/?address=${encodeURIComponent(inbox.inbox_id)}`}
                      className="font-mono text-sm font-medium break-all text-primary hover:underline"
                    >
                      {inbox.inbox_id}
                    </Link>
                    <p className="mt-1 text-sm text-muted">
                      {inbox.display_name || "No display name"} · created <When iso={inbox.created_at} />
                    </p>
                    {inbox.sending_paused_at ? (
                      <p className="mt-2">
                        <Badge tone="bad">sending paused</Badge>{" "}
                        <span className="text-xs text-muted">{inbox.sending_paused_reason}</span>
                      </p>
                    ) : null}
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Link
                      href={`/console/compose/?from=${encodeURIComponent(inbox.inbox_id)}`}
                      className="inline-flex min-h-11 items-center rounded-lg border border-border px-4 text-sm hover:border-fg/40"
                    >
                      Compose
                    </Link>
                    {inbox.ticket_intake ? (
                      <Badge tone="good">support queue</Badge>
                    ) : null}
                    {can("inbox_write") ? (
                      <Button busy={action.busy === `intake:${inbox.inbox_id}`} onClick={() => toggleIntake(inbox)}>
                        {inbox.ticket_intake ? "Intake off" : "Intake on"}
                      </Button>
                    ) : null}
                    {can("inbox_write") ? (
                      <ConfirmButton
                        label="Delete"
                        title={`Delete ${inbox.inbox_id}?`}
                        body={
                          <>
                            Its threads, messages and attachments are removed for good. The address is quarantined for
                            90 days: nobody can create it again in that time.
                          </>
                        }
                        confirmLabel="Delete inbox"
                        busy={action.busy === `delete:${inbox.inbox_id}`}
                        onConfirm={() => remove(inbox)}
                      />
                    ) : null}
                  </div>
                </li>
              ))}
            </ul>
            {list.data.next ? (
              <Button className="mt-4" busy={action.busy === "more"} onClick={() => list.data?.next && loadMore(list.data.next)}>
                Load more
              </Button>
            ) : null}
          </>
        )}
      </div>

      <div className="mt-6">
        <PushSettings />
      </div>
    </>
  );
}

function CreateInbox({ onCreated }: { onCreated: (inbox: Inbox) => void }) {
  const { session } = useConsole();
  const action = useAction();
  const [username, setUsername] = useState("");
  const [domain, setDomain] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState<string>();

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const parsed = CreateInboxInput.safeParse({
      username: username || undefined,
      domain: domain || undefined,
      display_name: displayName || undefined,
    });
    if (!parsed.success) {
      setError(parsed.error.issues[0]?.message);
      return;
    }
    setError(undefined);
    const out = await action.run(
      "create",
      () => call(session, "POST /v0/inboxes", { params: {}, body: parsed.data }, Inbox),
      { success: "Inbox created", failure: "Could not create the inbox" },
    );
    if (out.ok) {
      onCreated(out.value);
      setUsername("");
      setDisplayName("");
    }
  };

  return (
    <Card>
      <h2 className="font-display text-lg font-semibold">Create an inbox</h2>
      <form onSubmit={submit} noValidate className="mt-4 grid gap-4 sm:grid-cols-3">
        <Field label="Name" htmlFor="inbox-username" hint="Optional; random when empty.">
          <input id="inbox-username" value={username} onChange={(e) => setUsername(e.target.value)} className={inputClass} placeholder="support" autoCapitalize="none" spellCheck={false} />
        </Field>
        <Field label="Domain" htmlFor="inbox-domain" hint="Optional; a verified domain of yours, or the shared one.">
          <input id="inbox-domain" value={domain} onChange={(e) => setDomain(e.target.value)} className={inputClass} placeholder="mail.example.com" autoCapitalize="none" spellCheck={false} />
        </Field>
        <Field label="Display name" htmlFor="inbox-display" hint="Optional; shown in the From header.">
          <input id="inbox-display" value={displayName} onChange={(e) => setDisplayName(e.target.value)} className={inputClass} placeholder="Support agent" />
        </Field>
        <div className="flex flex-col gap-2 sm:col-span-3 sm:flex-row sm:items-center">
          <Button type="submit" variant="primary" busy={action.busy === "create"}>
            Create inbox
          </Button>
          {error ? (
            <p role="alert" className="text-sm text-danger">
              {error}
            </p>
          ) : null}
        </div>
      </form>
    </Card>
  );
}
