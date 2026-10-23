"use client";

import { useState, type FormEvent } from "react";
import { call, callNoContent } from "@/lib/console/client";
import { CreateWebhookInput, EVENT_TYPES, Webhook, WebhookList, type EventType } from "@/lib/console/schemas";
import { useAction, useResource } from "@/lib/console/use-resource";
import { useConsole } from "./ConsoleShell";
import { Badge, Button, Card, ConfirmButton, EmptyState, ErrorState, Field, inputClass, LoadingState, PageTitle, SecretOnce, When } from "./kit";
import { PermissionNote } from "./PermissionNote";

export function WebhooksView() {
  const { session, can } = useConsole();
  const action = useAction();
  const canRead = can("webhook_read");
  const hooks = useResource(
    canRead ? "webhooks" : null,
    (signal) => call(session, "GET /v0/webhooks", { params: {}, signal }, WebhookList),
    "webhooks",
  );
  const [created, setCreated] = useState<Webhook | null>(null);

  const remove = async (hook: Webhook): Promise<boolean> => {
    const out = await action.run(
      `delete:${hook.webhook_id}`,
      () => callNoContent(session, "DELETE /v0/webhooks/{webhook}", { params: { webhook: hook.webhook_id } }),
      { success: "Webhook deleted", failure: "Could not delete the webhook" },
    );
    if (out.ok) hooks.mutate((cur) => ({ ...cur, webhooks: cur.webhooks.filter((h) => h.webhook_id !== hook.webhook_id) }));
    return out.ok;
  };

  return (
    <>
      <PageTitle
        title="Webhooks"
        intro="Signed HTTPS callbacks for mail and domain events. Verify the signature with the secret shown when you create one."
      />
      {created?.secret ? (
        <div className="mb-6">
          <SecretOnce label={`Signing secret for ${created.url}`} value={created.secret} onDone={() => setCreated(null)} />
        </div>
      ) : null}
      {can("webhook_write") ? (
        <CreateWebhook
          onCreated={(h) => {
            setCreated(h);
            hooks.mutate((cur) => ({ ...cur, webhooks: [{ ...h, secret: undefined }, ...cur.webhooks] }));
          }}
        />
      ) : (
        <PermissionNote permission="webhook_write" action="create or delete webhooks" />
      )}
      <div className="mt-6">
        {!canRead ? (
          <PermissionNote permission="webhook_read" action="list webhooks" />
        ) : hooks.data === undefined ? (
          hooks.error ? (
            <ErrorState error={hooks.error} onRetry={hooks.reload} what="webhooks" />
          ) : (
            <LoadingState label="Loading webhooks" />
          )
        ) : hooks.data.webhooks.length === 0 ? (
          <EmptyState title="No webhooks yet">Create one above to be told when mail arrives.</EmptyState>
        ) : (
          <ul className="divide-y divide-border rounded-xl border border-border bg-card">
            {hooks.data.webhooks.map((h) => (
              <li key={h.webhook_id} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0">
                  <p className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-sm break-all">{h.url}</span>
                    <Badge tone={h.enabled ? "good" : "bad"}>{h.enabled ? "enabled" : "disabled"}</Badge>
                  </p>
                  <div className="mt-2 flex flex-wrap gap-1">
                    {h.event_types.map((e) => (
                      <Badge key={e}>{e}</Badge>
                    ))}
                  </div>
                  <p className="mt-2 text-xs break-words text-muted">
                    {h.inbox_ids.length ? `Inboxes: ${h.inbox_ids.join(", ")}` : "Every inbox"} · created <When iso={h.created_at} />
                  </p>
                </div>
                {can("webhook_write") ? (
                  <ConfirmButton
                    label="Delete"
                    title="Delete this webhook?"
                    body={<>Events stop being delivered to {h.url} at once.</>}
                    confirmLabel="Delete webhook"
                    busy={action.busy === `delete:${h.webhook_id}`}
                    onConfirm={() => remove(h)}
                  />
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>
    </>
  );
}

function CreateWebhook({ onCreated }: { onCreated: (h: Webhook) => void }) {
  const { session } = useConsole();
  const action = useAction();
  const [url, setUrl] = useState("");
  const [events, setEvents] = useState<EventType[]>(["message.received"]);
  const [inboxes, setInboxes] = useState("");
  const [errors, setErrors] = useState<Partial<Record<"url" | "event_types" | "inbox_ids", string>>>({});

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const parsed = CreateWebhookInput.safeParse({ url: url.trim(), event_types: events, inbox_ids: inboxes });
    if (!parsed.success) {
      const next: typeof errors = {};
      for (const issue of parsed.error.issues) {
        const f = issue.path[0];
        if (f === "url" || f === "event_types" || f === "inbox_ids") next[f] ??= issue.message;
      }
      setErrors(next);
      return;
    }
    setErrors({});
    const out = await action.run(
      "create",
      () => call(session, "POST /v0/webhooks", { params: {}, body: parsed.data }, Webhook),
      { success: "Webhook created; copy the secret now", failure: "Could not create the webhook" },
    );
    if (out.ok) {
      onCreated(out.value);
      setUrl("");
      setInboxes("");
    }
  };

  return (
    <Card>
      <h2 className="font-display text-lg font-semibold">Create a webhook</h2>
      <form onSubmit={submit} noValidate className="mt-4 flex flex-col gap-4">
        <Field label="Endpoint URL" htmlFor="hook-url" error={errors.url}>
          <input id="hook-url" type="url" inputMode="url" autoCapitalize="none" spellCheck={false} value={url} onChange={(e) => setUrl(e.target.value)} className={inputClass} placeholder="https://agent.example.com/hooks/mail" />
        </Field>
        <Field label="Only these inboxes" htmlFor="hook-inboxes" error={errors.inbox_ids} hint="Optional, up to 10 addresses separated by commas. Empty means every inbox.">
          <input id="hook-inboxes" inputMode="email" autoCapitalize="none" spellCheck={false} value={inboxes} onChange={(e) => setInboxes(e.target.value)} className={inputClass} />
        </Field>
        <fieldset>
          <legend className="text-sm font-medium">Events</legend>
          <div className="mt-2 grid gap-2 sm:grid-cols-2">
            {EVENT_TYPES.map((e) => (
              <label key={e} className="flex min-h-11 cursor-pointer items-center gap-3 rounded-lg border border-border px-3">
                <input
                  type="checkbox"
                  className="h-4 w-4"
                  checked={events.includes(e)}
                  onChange={(ev) => setEvents((cur) => (ev.target.checked ? [...cur, e] : cur.filter((x) => x !== e)))}
                />
                <span className="font-mono text-xs break-all">{e}</span>
              </label>
            ))}
          </div>
          {errors.event_types ? (
            <p role="alert" className="mt-2 text-sm text-danger">
              {errors.event_types}
            </p>
          ) : null}
        </fieldset>
        <div>
          <Button type="submit" variant="primary" busy={action.busy === "create"}>
            Create webhook
          </Button>
        </div>
      </form>
    </Card>
  );
}
