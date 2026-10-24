"use client";

import { useState } from "react";
import { call } from "@/lib/console/client";
import { SendResult, TicketDetail, TicketList, TicketNote, TicketStats } from "@/lib/console/schemas";
import type { Ticket, TicketPriority, TicketState, UpdateTicketBody } from "@/lib/console/schemas";
import { useAction, useResource } from "@/lib/console/use-resource";
import { useConsole } from "./ConsoleShell";
import { Badge, Button, Card, EmptyState, ErrorState, inputClass, LoadingState, PageTitle, When } from "./kit";

const STATES: readonly TicketState[] = ["new", "open", "pending", "resolved", "closed"];
const PRIORITIES: readonly TicketPriority[] = ["low", "normal", "high", "urgent"];

const stateTone: Record<TicketState, "neutral" | "good" | "warn" | "bad"> = {
  new: "warn",
  open: "neutral",
  pending: "warn",
  resolved: "good",
  closed: "neutral",
};

const priorityTone: Record<TicketPriority, "neutral" | "good" | "warn" | "bad"> = {
  low: "neutral",
  normal: "neutral",
  high: "warn",
  urgent: "bad",
};

/** The moves an agent or a person can make from the current state. */
const moves: Record<TicketState, { to: TicketState; label: string; variant: "primary" | "secondary" | "danger" }[]> = {
  new: [
    { to: "open", label: "Open", variant: "primary" },
    { to: "pending", label: "Waiting on customer", variant: "secondary" },
    { to: "resolved", label: "Resolved", variant: "secondary" },
    { to: "closed", label: "Close", variant: "danger" },
  ],
  open: [
    { to: "pending", label: "Waiting on customer", variant: "secondary" },
    { to: "resolved", label: "Resolved", variant: "primary" },
    { to: "closed", label: "Close", variant: "danger" },
  ],
  pending: [
    { to: "open", label: "Reopen", variant: "primary" },
    { to: "resolved", label: "Resolved", variant: "secondary" },
    { to: "closed", label: "Close", variant: "danger" },
  ],
  resolved: [
    { to: "open", label: "Reopen", variant: "secondary" },
    { to: "closed", label: "Close", variant: "danger" },
  ],
  closed: [{ to: "open", label: "Reopen", variant: "secondary" }],
};

export function TicketsView() {
  const { session } = useConsole();
  const [state, setState] = useState("");
  const [priority, setPriority] = useState("");
  const [unassigned, setUnassigned] = useState(false);
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState<string | null>(null);

  const filterKey = [state, priority, unassigned ? "none" : "", query].join("|");
  const list = useResource(
    `tickets:${filterKey}`,
    (signal) =>
      call(
        session,
        "GET /v0/tickets",
        {
          params: {},
          query: { state: state || undefined, priority: priority || undefined, assignee: unassigned ? "none" : undefined, q: query || undefined, limit: 50 },
          signal,
        },
        TicketList,
      ),
    "the queue",
  );
  const stats = useResource(`stats:${filterKey}`, (signal) => call(session, "GET /v0/tickets/stats", { params: {}, signal }, TicketStats), "the counters");

  return (
    <>
      <PageTitle
        title="Tickets"
        intro="A ticket is a thread with state. The message body is never copied, so reading a ticket and reading the mailbox always agree."
        actions={
          <Button onClick={() => { list.reload(); stats.reload(); }} busy={list.loading}>
            Refresh
          </Button>
        }
      />

      {stats.data ? (
        <div className="mb-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Count label="Open" value={stats.data.open} />
          <Count label="Breaching SLA" value={stats.data.breaching} tone={stats.data.breaching > 0 ? "bad" : "neutral"} />
          <Count label="Unassigned" value={stats.data.unassigned} />
          <Count label="Median first reply" value={stats.data.median_first_reply_seconds} unit="s" />
        </div>
      ) : null}

      <Card className="mb-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <label className="flex-1 text-sm">
            <span className="mb-1 block font-medium">Search</span>
            <input
              className={inputClass}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Subject or requester"
              type="search"
            />
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium">State</span>
            <select className={inputClass} value={state} onChange={(e) => setState(e.target.value)}>
              <option value="">Any state</option>
              {STATES.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-medium">Priority</span>
            <select className={inputClass} value={priority} onChange={(e) => setPriority(e.target.value)}>
              <option value="">Any priority</option>
              {PRIORITIES.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </label>
          <label className="flex min-h-11 items-center gap-2 text-sm">
            <input type="checkbox" checked={unassigned} onChange={(e) => setUnassigned(e.target.checked)} />
            Unassigned only
          </label>
        </div>
      </Card>

      {list.error ? <ErrorState error={list.error} onRetry={list.reload} what="the queue" /> : null}

      <div className="grid gap-4 lg:grid-cols-5">
        <div className="lg:col-span-2">
          {list.loading && !list.data ? <LoadingState label="Loading the queue" /> : null}
          {list.data && list.data.tickets.length === 0 ? (
            <EmptyState title="No tickets match">
              Mark an inbox as intake so new threads become tickets, or open one over the API.
            </EmptyState>
          ) : null}
          <ul className="space-y-2">
            {(list.data?.tickets ?? []).map((t) => (
              <li key={t.id}>
                <button
                  type="button"
                  onClick={() => setOpen(t.id)}
                  aria-current={open === t.id ? "true" : undefined}
                  className={`w-full rounded-xl border p-3 text-left transition-colors ${
                    open === t.id ? "border-fg/40 bg-card" : "border-border bg-card hover:border-fg/20"
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-mono text-xs text-muted">T-{t.number}</span>
                    <span className="flex items-center gap-1">
                      <Badge tone={priorityTone[t.priority]}>{t.priority}</Badge>
                      <Badge tone={stateTone[t.state]}>{t.state}</Badge>
                    </span>
                  </div>
                  <p className="mt-1 truncate font-medium">{t.subject || "(no subject)"}</p>
                  <p className="truncate text-sm text-muted">{t.requester}</p>
                  <p className="mt-1 text-xs text-muted">
                    {t.first_reply_at ? `Answered ${relative(t.first_reply_at)}` : dueLabel(t)}
                  </p>
                </button>
              </li>
            ))}
          </ul>
        </div>
        <div className="lg:col-span-3">
          <TicketPane id={open} onChanged={() => { list.reload(); stats.reload(); }} />
        </div>
      </div>
    </>
  );
}

function Count({ label, value, unit = "", tone = "neutral" }: { label: string; value: number; unit?: string; tone?: "neutral" | "bad" }) {
  return (
    <div className="rounded-xl border border-border bg-card p-3">
      <p className="text-xs uppercase tracking-wide text-muted">{label}</p>
      <p className={`mt-1 font-display text-xl font-semibold ${tone === "bad" && value > 0 ? "text-danger" : ""}`}>
        {value}
        {unit}
      </p>
    </div>
  );
}

function dueLabel(t: Ticket): string {
  if (!t.due_first_reply) return "No SLA on this plan";
  const due = new Date(t.due_first_reply).getTime();
  const late = due < Date.now();
  return `${late ? "First reply overdue" : "First reply due"} ${relative(t.due_first_reply)}`;
}

function relative(iso: string): string {
  const diff = new Date(iso).getTime() - Date.now();
  const abs = Math.abs(diff);
  const mins = Math.round(abs / 60000);
  const text = mins < 60 ? `${mins} min` : mins < 1440 ? `${Math.round(mins / 60)} h` : `${Math.round(mins / 1440)} d`;
  return diff < 0 ? `${text} ago` : `in ${text}`;
}

function TicketPane({ id, onChanged }: { id: string | null; onChanged: () => void }) {
  const { session } = useConsole();
  const action = useAction();
  const [note, setNote] = useState("");
  const [reply, setReply] = useState("");

  const detail = useResource(
    id ? `ticket:${id}` : null,
    (signal) => call(session, "GET /v0/tickets/{ticket}", { params: { ticket: id ?? "" }, signal }, TicketDetail),
    "the ticket",
  );

  if (!id) {
    return <EmptyState title="Pick a ticket">The thread opens beside the queue, with its internal notes.</EmptyState>;
  }
  if (detail.loading && !detail.data) return <LoadingState label="Opening the ticket" />;
  if (detail.error) return <ErrorState error={detail.error} onRetry={detail.reload} what="the ticket" />;
  const data = detail.data;
  if (!data) return null;
  const t = data.ticket;

  const change = async (body: UpdateTicketBody, id_: string, done: string) => {
    const out = await action.run(
      id_,
      () => call(session, "PATCH /v0/tickets/{ticket}", { params: { ticket: t.id }, body }, TicketDetail.shape.ticket),
      { success: done, failure: "Could not update the ticket" },
    );
    if (out.ok) {
      detail.reload();
      onChanged();
    }
  };

  const addNote = async () => {
    const body = note.trim();
    if (!body) return;
    const out = await action.run(
      "note",
      () => call(session, "POST /v0/tickets/{ticket}/notes", { params: { ticket: t.id }, body: { body } }, TicketNote),
      { success: "Note saved", failure: "Could not save the note" },
    );
    if (out.ok) {
      setNote("");
      detail.reload();
    }
  };

  const sendReply = async () => {
    const text = reply.trim();
    if (!text) return;
    const out = await action.run(
      "reply",
      () => call(session, "POST /v0/tickets/{ticket}/reply", { params: { ticket: t.id }, body: { text, reply_all: false } }, SendResult),
      { success: "Reply sent", failure: "Could not send the reply" },
    );
    if (out.ok) {
      setReply("");
      detail.reload();
      onChanged();
    }
  };

  return (
    <div className="space-y-4">
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="min-w-0">
            <p className="font-mono text-xs text-muted">T-{t.number}</p>
            <h2 className="font-display text-lg font-semibold break-words">{t.subject || "(no subject)"}</h2>
            <p className="text-sm text-muted">{t.requester}</p>
          </div>
          <div className="flex flex-wrap items-center gap-1">
            <Badge tone={priorityTone[t.priority]}>{t.priority}</Badge>
            <Badge tone={stateTone[t.state]}>{t.state}</Badge>
          </div>
        </div>
        <p className="mt-2 text-xs text-muted">{dueLabel(t)}</p>
        <div className="mt-4 flex flex-wrap gap-2">
          {moves[t.state].map((m) => (
            <Button
              key={m.to}
              variant={m.variant}
              busy={action.busy === `state:${m.to}`}
              onClick={() => change({ state: m.to }, `state:${m.to}`, `Moved to ${m.to}`)}
            >
              {m.label}
            </Button>
          ))}
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <label className="text-sm text-muted" htmlFor="priority">
            Priority
          </label>
          <select
            id="priority"
            className={`${inputClass} w-auto`}
            value={t.priority}
            onChange={(e) => change({ priority: e.target.value as TicketPriority }, "priority", "Priority updated")}
          >
            {PRIORITIES.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
          {t.assignee_user || t.assignee_key ? (
            <span className="text-xs text-muted">
              Assigned to <span className="font-mono">{(t.assignee_user || t.assignee_key || "").slice(0, 8)}</span>
            </span>
          ) : (
            <span className="text-xs text-muted">Unassigned</span>
          )}
        </div>
      </Card>

      <Card>
        <h3 className="font-medium">Reply to the customer</h3>
        <p className="mt-1 text-xs text-muted">Goes out on the same thread, through the normal sending path.</p>
        <textarea
          className={`${inputClass} mt-3 min-h-24`}
          value={reply}
          onChange={(e) => setReply(e.target.value)}
          placeholder="Your answer"
        />
        <div className="mt-2 flex justify-end">
          <Button variant="primary" onClick={sendReply} busy={action.busy === "reply"} disabled={!reply.trim()}>
            Send reply
          </Button>
        </div>
      </Card>

      <Card>
        <h3 className="font-medium">Internal note</h3>
        <p className="mt-1 text-xs text-muted">Stored separately from the thread. It can never be emailed to the customer.</p>
        <textarea
          className={`${inputClass} mt-3 min-h-20`}
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder="What the next person needs to know"
        />
        <div className="mt-2 flex justify-end">
          <Button onClick={addNote} busy={action.busy === "note"} disabled={!note.trim()}>
            Save note
          </Button>
        </div>
      </Card>

      <Card>
        <h3 className="font-medium">Timeline</h3>
        <ul className="mt-3 space-y-3">
          {timeline(data).map((row) => (
            <li key={row.key} className="border-l-2 border-border pl-3">
              <p className="text-sm">
                <span className="font-medium">{row.title}</span>{" "}
                <span className="font-mono text-xs text-muted">{row.actor}</span>
              </p>
              {row.body ? <p className="mt-1 whitespace-pre-wrap text-sm text-muted">{row.body}</p> : null}
              <p className="mt-1 text-xs text-muted">
                <When iso={row.at} />
              </p>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}

function timeline(data: TicketDetail): { key: string; title: string; actor: string; body?: string; at: string }[] {
  const rows = [
    ...data.events.map((e) => ({
      key: `e${e.id}`,
      title: e.kind,
      actor: e.actor,
      at: e.created_at,
      body: detailText(e.detail),
    })),
    ...data.notes.map((n) => ({
      key: `n${n.id}`,
      title: "note",
      actor: n.author,
      at: n.created_at,
      body: n.body,
    })),
  ];
  return rows.sort((a, b) => a.at.localeCompare(b.at));
}

function detailText(detail: unknown): string | undefined {
  if (!detail || typeof detail !== "object") return undefined;
  const parts = Object.entries(detail as Record<string, unknown>)
    .filter(([, v]) => v !== "" && v !== null && v !== undefined)
    .map(([k, v]) => `${k}: ${Array.isArray(v) ? v.join(", ") : String(v)}`);
  return parts.length ? parts.join(", ") : undefined;
}
