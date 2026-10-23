"use client";

import { useState, type FormEvent } from "react";
import { call } from "@/lib/console/client";
import { Domain, DomainInput, DomainList } from "@/lib/console/schemas";
import { useAction, useResource } from "@/lib/console/use-resource";
import { useConsole } from "./ConsoleShell";
import { Badge, Button, Card, EmptyState, ErrorState, Field, inputClass, LoadingState, PageTitle, When } from "./kit";
import { PermissionNote } from "./PermissionNote";
import { useToast } from "./Toast";

export function DomainsView() {
  const { session, can } = useConsole();
  const action = useAction();
  const toast = useToast();
  const canRead = can("domain_read");
  const domains = useResource(
    canRead ? "domains" : null,
    (signal) => call(session, "GET /v0/domains", { params: {}, signal }, DomainList),
    "domains",
  );

  const upsert = (d: Domain) =>
    domains.mutate((cur) => {
      const exists = cur.domains.some((x) => x.domain_id === d.domain_id);
      return { ...cur, domains: exists ? cur.domains.map((x) => (x.domain_id === d.domain_id ? d : x)) : [d, ...cur.domains] };
    });

  const verify = async (d: Domain) => {
    const out = await action.run(
      `verify:${d.domain}`,
      () => call(session, "POST /v0/domains/{domain}/verify", { params: { domain: d.domain } }, Domain),
      { failure: `Could not check ${d.domain}` },
    );
    if (!out.ok) return;
    upsert(out.value);
    const missing = out.value.records.filter((r) => r.status === "missing").length;
    if (out.value.status === "verified") {
      toast.success(`${out.value.domain} is verified`, "Inboxes can now use it.");
    } else {
      toast.info(
        `${out.value.domain} is not verified yet`,
        `${missing} record${missing === 1 ? "" : "s"} still missing. DNS changes can take a while to show up.`,
      );
    }
  };

  return (
    <>
      <PageTitle
        title="Domains"
        intro="Send and receive on your own domain. Publish the DNS records shown, then check. The platform also checks every 5 minutes for 7 days."
      />
      {can("domain_write") ? <AddDomain onAdded={upsert} /> : <PermissionNote permission="domain_write" action="add or verify domains" />}
      <div className="mt-6">
        {!canRead ? (
          <PermissionNote permission="domain_read" action="list domains" />
        ) : domains.data === undefined ? (
          domains.error ? (
            <ErrorState error={domains.error} onRetry={domains.reload} what="domains" />
          ) : (
            <LoadingState label="Loading domains" />
          )
        ) : domains.data.domains.length === 0 ? (
          <EmptyState title="No domains yet">Add one above to get the DNS records to publish.</EmptyState>
        ) : (
          <ul className="flex flex-col gap-4">
            {domains.data.domains.map((d) => (
              <li key={d.domain_id}>
                <Card>
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div className="min-w-0">
                      <p className="flex flex-wrap items-center gap-2">
                        <span className="font-mono font-medium break-all">{d.domain}</span>
                        <Badge tone={d.status === "verified" ? "good" : "warn"}>{d.status}</Badge>
                        {d.shared ? <Badge>shared</Badge> : null}
                      </p>
                      <p className="mt-1 text-xs text-muted">
                        Last checked <When iso={d.last_checked_at} />
                        {d.verified_at ? (
                          <>
                            {" "}
                            · verified <When iso={d.verified_at} />
                          </>
                        ) : null}
                      </p>
                    </div>
                    {!d.shared && can("domain_write") ? (
                      <Button onClick={() => verify(d)} busy={action.busy === `verify:${d.domain}`}>
                        Check DNS now
                      </Button>
                    ) : null}
                  </div>
                  {d.records.length ? (
                    <div className="mt-4 flex flex-col gap-2">
                      {d.records.map((r) => (
                        <div key={`${r.type}|${r.name}|${r.value}`} className="rounded-lg border border-border bg-bg p-3">
                          <p className="flex flex-wrap items-center gap-2 text-xs">
                            <Badge>{r.type}</Badge>
                            {r.status ? <Badge tone={r.status === "verified" ? "good" : "bad"}>{r.status}</Badge> : null}
                          </p>
                          <dl className="mt-2 grid gap-1 text-xs sm:grid-cols-[6rem_minmax(0,1fr)]">
                            <dt className="text-muted">Name</dt>
                            <dd className="font-mono break-all">{r.name}</dd>
                            <dt className="text-muted">Value</dt>
                            <dd className="font-mono break-all">{r.value}</dd>
                          </dl>
                        </div>
                      ))}
                    </div>
                  ) : null}
                </Card>
              </li>
            ))}
          </ul>
        )}
      </div>
    </>
  );
}

function AddDomain({ onAdded }: { onAdded: (d: Domain) => void }) {
  const { session } = useConsole();
  const action = useAction();
  const [name, setName] = useState("");
  const [error, setError] = useState<string>();

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const parsed = DomainInput.safeParse(name);
    if (!parsed.success) {
      setError(parsed.error.issues[0]?.message);
      return;
    }
    setError(undefined);
    const out = await action.run(
      "add",
      () => call(session, "POST /v0/domains", { params: {}, body: { domain: parsed.data } }, Domain),
      { success: `Added ${parsed.data}; publish the records below`, failure: `Could not add ${parsed.data}` },
    );
    if (out.ok) {
      onAdded(out.value);
      setName("");
    }
  };

  return (
    <Card>
      <h2 className="font-display text-lg font-semibold">Add a domain</h2>
      <form onSubmit={submit} noValidate className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-start">
        <div className="flex-1">
          <Field label="Domain" htmlFor="domain-name" error={error}>
            <input id="domain-name" value={name} onChange={(e) => setName(e.target.value)} className={inputClass} placeholder="mail.example.com" autoCapitalize="none" spellCheck={false} />
          </Field>
        </div>
        <Button type="submit" variant="primary" busy={action.busy === "add"} className="sm:mt-7">
          Add domain
        </Button>
      </form>
    </Card>
  );
}
