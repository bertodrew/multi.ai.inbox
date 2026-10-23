"use client";

import { useState, type FormEvent } from "react";
import { call, callNoContent } from "@/lib/console/client";
import { ApiKey, ApiKeyList, CreateKeyInput, PERMISSIONS, type Permission } from "@/lib/console/schemas";
import { useAction, useResource } from "@/lib/console/use-resource";
import { useConsole } from "./ConsoleShell";
import { Badge, Button, Card, ConfirmButton, EmptyState, ErrorState, Field, inputClass, LoadingState, PageTitle, SecretOnce, When } from "./kit";
import { PermissionNote } from "./PermissionNote";

const meaning: Record<Permission, string> = {
  inbox_read: "List and get inboxes",
  inbox_write: "Create, rename and delete inboxes",
  message_read: "Read threads, messages and attachments",
  message_send: "Send messages and replies",
  message_update: "Add and remove labels",
  domain_read: "List and get domains",
  domain_write: "Add, verify and delete domains",
  webhook_read: "List webhooks",
  webhook_write: "Create and delete webhooks",
  api_key_read: "List API keys",
  api_key_write: "Create and revoke API keys",
  audit_read: "Read the audit log",
  message_review: "Read mail held for review",
  message_release: "Release mail held for review; only a key that lists it can grant it",
  billing_read: "See the plan, billing status and renewal date",
  billing_write: "Open Stripe checkout and the billing portal",
};

export function KeysView() {
  const { session, me, can, signOut } = useConsole();
  const action = useAction();
  const canRead = can("api_key_read");
  const keys = useResource(
    canRead ? "keys" : null,
    (signal) => call(session, "GET /v0/api-keys", { params: {}, signal }, ApiKeyList),
    "API keys",
  );
  const [created, setCreated] = useState<ApiKey | null>(null);

  const revoke = async (key: ApiKey): Promise<boolean> => {
    const self = key.api_key_id === me.api_key_id;
    const out = await action.run(
      `revoke:${key.api_key_id}`,
      () => callNoContent(session, "DELETE /v0/api-keys/{key}", { params: { key: key.api_key_id } }),
      { success: `Revoked ${key.name || key.prefix}`, failure: `Could not revoke ${key.name || key.prefix}` },
    );
    if (out.ok) {
      keys.mutate((cur) => ({ ...cur, api_keys: cur.api_keys.filter((k) => k.api_key_id !== key.api_key_id) }));
      if (self) signOut();
    }
    return out.ok;
  };

  return (
    <>
      <PageTitle
        title="API keys"
        intro="Give each agent its own key with only the permissions it needs. A key can grant at most what it holds itself."
      />
      {created?.api_key ? (
        <div className="mb-6">
          <SecretOnce label={`New key ${created.name || created.prefix}`} value={created.api_key} onDone={() => setCreated(null)} />
        </div>
      ) : null}
      {can("api_key_write") ? (
        <CreateKey
          onCreated={(k) => {
            setCreated(k);
            keys.mutate((cur) => ({ ...cur, api_keys: [{ ...k, api_key: undefined }, ...cur.api_keys] }));
          }}
        />
      ) : (
        <PermissionNote permission="api_key_write" action="create or revoke keys" />
      )}
      <div className="mt-6">
        {!canRead ? (
          <PermissionNote permission="api_key_read" action="list keys" />
        ) : keys.data === undefined ? (
          keys.error ? (
            <ErrorState error={keys.error} onRetry={keys.reload} what="API keys" />
          ) : (
            <LoadingState label="Loading API keys" />
          )
        ) : keys.data.api_keys.length === 0 ? (
          <EmptyState title="No active keys">Create one above.</EmptyState>
        ) : (
          <ul className="divide-y divide-border rounded-xl border border-border bg-card">
            {keys.data.api_keys.map((k) => {
              const self = k.api_key_id === me.api_key_id;
              return (
                <li key={k.api_key_id} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-start sm:justify-between">
                  <div className="min-w-0">
                    <p className="flex flex-wrap items-center gap-2">
                      <span className="font-medium break-words">{k.name || "Unnamed key"}</span>
                      <code className="font-mono text-xs text-muted">{k.prefix}…</code>
                      {self ? <Badge tone="good">this session</Badge> : null}
                    </p>
                    <div className="mt-2 flex flex-wrap gap-1">
                      {k.permissions === null ? (
                        <Badge tone="warn">full access</Badge>
                      ) : (
                        k.permissions.map((p) => <Badge key={p}>{p}</Badge>)
                      )}
                    </div>
                    <p className="mt-2 text-xs text-muted">
                      Created <When iso={k.created_at} /> · last used <When iso={k.last_used_at} /> · expires{" "}
                      {k.expires_at ? <When iso={k.expires_at} /> : "never"}
                    </p>
                  </div>
                  {can("api_key_write") ? (
                    <ConfirmButton
                      label="Revoke"
                      title={`Revoke ${k.name || k.prefix}?`}
                      body={
                        self
                          ? "This is the key you are signed in with. It stops working at once and you are signed out."
                          : "Every agent using this key loses access at once. This cannot be undone."
                      }
                      confirmLabel="Revoke key"
                      busy={action.busy === `revoke:${k.api_key_id}`}
                      onConfirm={() => revoke(k)}
                    />
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </>
  );
}

function CreateKey({ onCreated }: { onCreated: (k: ApiKey) => void }) {
  const { session, can } = useConsole();
  const action = useAction();
  const held = PERMISSIONS.filter((p) => can(p));
  const [name, setName] = useState("");
  const [perms, setPerms] = useState<Permission[]>([]);
  const [expires, setExpires] = useState("");
  const [errors, setErrors] = useState<Partial<Record<"name" | "permissions" | "expires_at", string>>>({});

  const toggle = (p: Permission, on: boolean) => setPerms((cur) => (on ? [...cur, p] : cur.filter((x) => x !== p)));

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const parsed = CreateKeyInput.safeParse({ name, permissions: perms, expires_at: expires });
    if (!parsed.success) {
      const next: typeof errors = {};
      for (const issue of parsed.error.issues) {
        const f = issue.path[0];
        if (f === "name" || f === "permissions" || f === "expires_at") next[f] ??= issue.message;
      }
      setErrors(next);
      return;
    }
    setErrors({});
    const out = await action.run(
      "create",
      () => call(session, "POST /v0/api-keys", { params: {}, body: parsed.data }, ApiKey),
      { success: "Key created; copy it now", failure: "Could not create the key" },
    );
    if (out.ok) {
      onCreated(out.value);
      setName("");
      setPerms([]);
      setExpires("");
    }
  };

  return (
    <Card>
      <h2 className="font-display text-lg font-semibold">Create a scoped key</h2>
      <form onSubmit={submit} noValidate className="mt-4 flex flex-col gap-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Name" htmlFor="key-name" error={errors.name} hint="For example the agent that will use it.">
            <input id="key-name" value={name} onChange={(e) => setName(e.target.value)} className={inputClass} maxLength={100} />
          </Field>
          <Field label="Expires" htmlFor="key-expires" error={errors.expires_at} hint="Optional. A key never outlives the key that created it.">
            <input id="key-expires" type="datetime-local" value={expires} onChange={(e) => setExpires(e.target.value)} className={inputClass} />
          </Field>
        </div>
        <fieldset>
          <legend className="text-sm font-medium">Permissions</legend>
          <div className="mt-2 flex flex-wrap gap-2">
            <Button variant="ghost" className="px-2" onClick={() => setPerms([...held])}>
              Select all this key holds
            </Button>
            <Button variant="ghost" className="px-2" onClick={() => setPerms([])}>
              Clear
            </Button>
          </div>
          <div className="mt-2 grid gap-2 sm:grid-cols-2">
            {PERMISSIONS.map((p) => {
              const allowed = held.includes(p);
              return (
                <label
                  key={p}
                  className={`flex min-h-11 items-start gap-3 rounded-lg border border-border px-3 py-2 ${allowed ? "cursor-pointer" : "opacity-60"}`}
                >
                  <input
                    type="checkbox"
                    className="mt-1 h-4 w-4"
                    checked={perms.includes(p)}
                    disabled={!allowed}
                    onChange={(e) => toggle(p, e.target.checked)}
                  />
                  <span className="min-w-0">
                    <span className="block font-mono text-xs">{p}</span>
                    <span className="block text-xs text-muted">
                      {meaning[p]}
                      {allowed ? "" : ". Your key does not hold it."}
                    </span>
                  </span>
                </label>
              );
            })}
          </div>
          {errors.permissions ? (
            <p role="alert" className="mt-2 text-sm text-danger">
              {errors.permissions}
            </p>
          ) : null}
        </fieldset>
        <div>
          <Button type="submit" variant="primary" busy={action.busy === "create"}>
            Create key
          </Button>
        </div>
      </form>
    </Card>
  );
}
