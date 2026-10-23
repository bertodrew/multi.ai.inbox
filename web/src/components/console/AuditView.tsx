"use client";

import { useState } from "react";
import { call } from "@/lib/console/client";
import { AuditList } from "@/lib/console/schemas";
import { useResource } from "@/lib/console/use-resource";
import { useConsole } from "./ConsoleShell";
import { Button, EmptyState, ErrorState, inputClass, LoadingState, PageTitle, When } from "./kit";
import { PermissionNote } from "./PermissionNote";

const LIMITS = [50, 100, 200] as const;

function metadataText(meta: unknown): string {
  if (meta === null || meta === undefined) return "";
  try {
    return JSON.stringify(meta, null, 2);
  } catch {
    return String(meta);
  }
}

export function AuditView() {
  const { session, can } = useConsole();
  const allowed = can("audit_read");
  const [limit, setLimit] = useState<(typeof LIMITS)[number]>(50);
  const log = useResource(
    allowed ? `audit|${limit}` : null,
    (signal) => call(session, "GET /v0/audit-logs", { params: {}, query: { limit }, signal }, AuditList),
    "the audit log",
  );

  return (
    <>
      <PageTitle
        title="Audit log"
        intro="Security relevant actions in your organization: keys, inboxes, webhooks, domains and releases."
        actions={
          allowed ? (
            <>
              <label htmlFor="audit-limit" className="sr-only">
                Entries to show
              </label>
              <select
                id="audit-limit"
                value={limit}
                onChange={(e) => {
                  const n = Number(e.target.value);
                  const found = LIMITS.find((l) => l === n);
                  if (found) setLimit(found);
                }}
                className={`${inputClass} w-auto`}
              >
                {LIMITS.map((l) => (
                  <option key={l} value={l}>
                    Last {l}
                  </option>
                ))}
              </select>
              <Button onClick={log.reload} busy={log.loading && log.data !== undefined}>
                Refresh
              </Button>
            </>
          ) : null
        }
      />
      {!allowed ? (
        <PermissionNote permission="audit_read" action="read the audit log" />
      ) : log.data === undefined ? (
        log.error ? (
          <ErrorState error={log.error} onRetry={log.reload} what="the audit log" />
        ) : (
          <LoadingState label="Loading the audit log" />
        )
      ) : log.data.entries.length === 0 ? (
        <EmptyState title="No entries yet">Actions such as creating a key or deleting an inbox are recorded here.</EmptyState>
      ) : (
        <ol className="divide-y divide-border rounded-xl border border-border bg-card">
          {log.data.entries.map((e) => {
            const meta = metadataText(e.metadata);
            return (
              <li key={e.id} className="p-4">
                <div className="flex flex-col gap-1 sm:flex-row sm:items-baseline sm:justify-between">
                  <p className="min-w-0 text-sm">
                    <span className="font-mono font-medium">{e.action}</span>
                    {e.target ? <span className="font-mono break-all text-muted"> {e.target}</span> : null}
                  </p>
                  <p className="shrink-0 text-xs text-muted">
                    <When iso={e.created_at} />
                  </p>
                </div>
                <p className="mt-1 text-xs break-all text-muted">
                  by {e.actor} from {e.ip || "unknown address"} · request {e.request_id}
                </p>
                {meta ? (
                  <details className="mt-2">
                    <summary className="cursor-pointer text-xs text-primary">Details</summary>
                    <pre className="mt-2 overflow-x-auto rounded-lg bg-code-bg p-3 font-mono text-xs text-code-fg">{meta}</pre>
                  </details>
                ) : null}
              </li>
            );
          })}
        </ol>
      )}
    </>
  );
}
