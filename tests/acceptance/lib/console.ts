// Console and API helpers shared by the acceptance specs.

import type { APIRequestContext, BrowserContext, Page } from "@playwright/test";
import { API_KEY, BASE_URL, SESSION_KEY } from "./env";

export type Json = Record<string, unknown>;

/** Puts a session in the tab's sessionStorage the way the console itself does. */
export async function seedSession(context: BrowserContext, apiKey = API_KEY, apiBase = BASE_URL): Promise<void> {
  const value = JSON.stringify({ apiKey, apiBase });
  await context.addInitScript(
    ([key, session]: [string, string]) => {
      try {
        window.sessionStorage.setItem(key, session);
      } catch {
        // storage blocked: the spec that needs it will see the gate instead
      }
    },
    [SESSION_KEY, value] as [string, string],
  );
}

/** Opens the console already signed in. */
export async function openConsole(page: Page, path = "/console/"): Promise<void> {
  await page.goto(`${BASE_URL}${path}`, { waitUntil: "domcontentloaded" });
}

export function unique(prefix: string): string {
  return `${prefix}-${Date.now().toString(36)}-${Math.floor(Math.random() * 1e4)}`;
}

/** Raw API call with the bearer key, used only to build fixtures. */
export async function api(
  request: APIRequestContext,
  method: "GET" | "POST" | "PATCH" | "DELETE",
  path: string,
  body?: unknown,
): Promise<{ status: number; body: Json | null }> {
  const response = await request.fetch(`${BASE_URL}${path}`, {
    method,
    headers: {
      authorization: `Bearer ${API_KEY}`,
      "content-type": "application/json",
    },
    data: body === undefined ? undefined : JSON.stringify(body),
    failOnStatusCode: false,
  });
  const text = await response.text();
  let parsed: Json | null = null;
  try {
    parsed = text === "" ? null : (JSON.parse(text) as Json);
  } catch {
    parsed = null;
  }
  return { status: response.status(), body: parsed };
}

/** Polls a promise until it returns something truthy, or gives up. */
export async function poll<T>(fn: () => Promise<T | null | undefined>, timeoutMs = 60_000, everyMs = 2_000): Promise<T | null> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await fn();
    if (value) return value;
    if (Date.now() > deadline) return null;
    await new Promise((resolve) => setTimeout(resolve, everyMs));
  }
}

export type InboxSummary = { inbox_id: string; id: string; ticket_intake?: boolean; display_name?: string };

export async function listInboxes(request: APIRequestContext): Promise<InboxSummary[]> {
  const res = await api(request, "GET", "/v0/inboxes?limit=100");
  const inboxes = (res.body?.inboxes ?? []) as InboxSummary[];
  return inboxes;
}

/** Creates an inbox over the API so a spec can start from a known address. */
export async function createInbox(request: APIRequestContext, prefix: string): Promise<InboxSummary | null> {
  const name = unique(prefix);
  const res = await api(request, "POST", "/v0/inboxes", { username: name, display_name: name });
  if (res.status !== 200 && res.status !== 201) return null;
  return res.body as unknown as InboxSummary;
}

export async function deleteInbox(request: APIRequestContext, inbox: string): Promise<void> {
  await api(request, "DELETE", `/v0/inboxes/${encodeURIComponent(inbox)}`);
}

export async function setIntake(request: APIRequestContext, inbox: string, on: boolean): Promise<boolean> {
  const res = await api(request, "PATCH", `/v0/inboxes/${encodeURIComponent(inbox)}`, { ticket_intake: on });
  return res.status === 200;
}

export type ThreadSummary = { thread_id: string; subject: string; messages: Message[] };

export async function listThreads(request: APIRequestContext, inbox: string): Promise<ThreadSummary[]> {
  const res = await api(request, "GET", `/v0/inboxes/${encodeURIComponent(inbox)}/threads?limit=50`);
  const threads = (res.body?.threads ?? []) as ThreadSummary[];
  return threads;
}

export type Message = {
  id: string;
  subject: string;
  html?: string | null;
  text?: string | null;
  attachments?: { attachment_id: string; filename: string; size: number }[];
};

export async function getThreadMessages(request: APIRequestContext, thread: string): Promise<Message[]> {
  const res = await api(request, "GET", `/v0/threads/${encodeURIComponent(thread)}`);
  const messages = (res.body?.messages ?? []) as Message[];
  return messages;
}

/** Sends a message over the API, which is the only way to place hostile HTML in a mailbox. */
export async function sendMail(
  request: APIRequestContext,
  from: string,
  to: string,
  subject: string,
  options: { text?: string; html?: string; attachments?: { filename: string; content_type: string; content: string }[] } = {},
): Promise<{ ok: boolean; thread?: string; message?: string; status: number }> {
  const res = await api(request, "POST", `/v0/inboxes/${encodeURIComponent(from)}/messages/send`, {
    to: [to],
    subject,
    text: options.text ?? subject,
    html: options.html,
    attachments: options.attachments,
  });
  return {
    ok: res.status === 200,
    thread: typeof res.body?.thread_id === "string" ? res.body.thread_id : undefined,
    message: typeof res.body?.message_id === "string" ? res.body.message_id : undefined,
    status: res.status,
  };
}

/** Waits for a thread with this subject to land in the inbox. */
export async function waitForThread(
  request: APIRequestContext,
  inbox: string,
  subject: string,
  timeoutMs = 90_000,
): Promise<ThreadSummary | null> {
  return poll(async () => {
    const threads = await listThreads(request, inbox);
    return threads.find((t) => t.subject === subject) ?? null;
  }, timeoutMs);
}

export type Ticket = { id: string; number: number; subject: string; state: string; priority: string };

export async function listTickets(request: APIRequestContext, query = "?limit=50"): Promise<Ticket[]> {
  const res = await api(request, "GET", `/v0/tickets${query}`);
  return (res.body?.tickets ?? []) as Ticket[];
}

export async function waitForTicket(request: APIRequestContext, subject: string, timeoutMs = 90_000): Promise<Ticket | null> {
  return poll(async () => {
    const tickets = await listTickets(request);
    return tickets.find((t) => t.subject === subject) ?? null;
  }, timeoutMs);
}

/** True when the page does not scroll sideways at its current width. */
export async function noHorizontalOverflow(page: Page): Promise<{ ok: boolean; detail: string }> {
  const metrics = await page.evaluate(() => {
    const doc = document.documentElement;
    return {
      scrollWidth: doc.scrollWidth,
      clientWidth: doc.clientWidth,
      innerWidth: window.innerWidth,
      widest: Array.from(document.querySelectorAll<HTMLElement>("body *"))
        .map((el) => {
          const rect = el.getBoundingClientRect();
          return { tag: el.tagName.toLowerCase(), right: Math.round(rect.right), width: Math.round(rect.width) };
        })
        .filter((el) => el.right > doc.clientWidth + 1)
        .slice(0, 5),
    };
  });
  const ok = metrics.scrollWidth <= metrics.clientWidth + 1;
  const detail = `scrollWidth ${metrics.scrollWidth} against clientWidth ${metrics.clientWidth}` +
    (metrics.widest.length > 0 ? `; widest: ${JSON.stringify(metrics.widest)}` : "");
  return { ok, detail };
}
