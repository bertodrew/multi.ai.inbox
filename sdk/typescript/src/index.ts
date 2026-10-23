import type {
  APIKey,
  AuditEntry,
  Billing,
  BillingSession,
  CheckoutRequest,
  CreateAPIKeyRequest,
  CreateDraftRequest,
  CreateInboxRequest,
  CreateTicketRequest,
  CreateWebhookRequest,
  DeleteOrganizationResult,
  Domain,
  Draft,
  DraftList,
  Event,
  EventEnvelope,
  EventOptions,
  EventType,
  Inbox,
  InboxList,
  ListOptions,
  Me,
  Message,
  MessageList,
  OAuthStart,
  OrgExport,
  ReplyRequest,
  SendRequest,
  SendResult,
  Session,
  Thread,
  ThreadList,
  Ticket,
  TicketDetail,
  TicketFilter,
  TicketList,
  TicketNote,
  TicketStats,
  UpdateDraftRequest,
  UpdateInboxRequest,
  UpdateTicketRequest,
  Webhook,
} from "./types.js";

export * from "./types.js";

/** Default production API. Pass `baseUrl` for staging or a self hosted deployment. */
export const DEFAULT_BASE_URL = "https://api.aimulti.inbox";

/** The API version the client speaks unless told otherwise. */
export const DEFAULT_VERSION = "v1";

export type Query = Record<string, string | number | boolean | undefined>;

export interface ClientOptions {
  /** Another deployment, for example a staging host. A trailing slash is fine. */
  baseUrl?: string;
  /** "v0" speaks the deprecated prefix, for a deployment that predates v1. */
  version?: "v1" | "v0";
  /** A fetch implementation. Defaults to the global one. */
  fetch?: typeof globalThis.fetch;
  /** The User-Agent header, when the runtime lets a client set it. */
  userAgent?: string;
}

/** A failed call: the status, the error code and the request id to quote. */
export class AimultiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId?: string;
  /** Seconds the API asked the caller to wait, from Retry-After. */
  readonly retryAfter?: number;
  readonly method: string;
  readonly url: string;

  constructor(init: {
    status: number;
    code: string;
    message: string;
    method: string;
    url: string;
    requestId?: string;
    retryAfter?: number;
  }) {
    super(init.message);
    this.name = "AimultiError";
    this.status = init.status;
    this.code = init.code;
    this.requestId = init.requestId;
    this.retryAfter = init.retryAfter;
    this.method = init.method;
    this.url = init.url;
  }
}

/** True when err is an API error with this code, for example "already_exists". */
export function isCode(err: unknown, code: string): boolean {
  return err instanceof AimultiError && err.code === code;
}

interface RequestOptions {
  query?: Query;
  body?: unknown;
  idempotencyKey?: string;
  accept?: string;
}

/** A downloaded file, with the metadata the API reported. */
export interface DownloadedFile {
  filename: string;
  contentType: string;
  data: Uint8Array;
}

export class AimultiClient {
  private readonly apiKey: string;
  private readonly baseUrl: string;
  private readonly version: string;
  private readonly fetchImpl: typeof globalThis.fetch;
  private readonly userAgent?: string;

  /**
   * A client for one API key. The key can be a `mai_live_` key for an agent or a
   * session access token for a person.
   */
  constructor(apiKey: string, options: ClientOptions = {}) {
    this.apiKey = apiKey;
    this.baseUrl = (options.baseUrl ?? DEFAULT_BASE_URL).replace(/\/$/, "");
    this.version = options.version ?? DEFAULT_VERSION;
    this.userAgent = options.userAgent;
    const impl = options.fetch ?? globalThis.fetch;
    if (typeof impl !== "function") {
      throw new Error("aimulti: no fetch available, pass one as the fetch option");
    }
    this.fetchImpl = impl;
  }

  // ---- transport ----

  private url(path: string, query?: Query): string {
    const u = new URL(`${this.baseUrl}/${this.version}${path}`);
    for (const [key, value] of Object.entries(query ?? {})) {
      if (value === undefined || value === "") continue;
      u.searchParams.set(key, String(value));
    }
    return u.toString();
  }

  private headers(init: RequestOptions): Headers {
    const h = new Headers({ Authorization: `Bearer ${this.apiKey}`, Accept: init.accept ?? "application/json" });
    if (this.userAgent) h.set("User-Agent", this.userAgent);
    if (init.body !== undefined) h.set("Content-Type", "application/json");
    if (init.idempotencyKey) h.set("Idempotency-Key", init.idempotencyKey);
    return h;
  }

  private async failure(res: Response, method: string, url: string): Promise<AimultiError> {
    let code = "internal_error";
    let message = res.statusText || `HTTP ${res.status}`;
    try {
      const envelope = (await res.json()) as { code?: string; message?: string };
      if (envelope.code) code = envelope.code;
      if (envelope.message) message = envelope.message;
    } catch {
      // not the JSON envelope; keep the status line
    }
    const retry = Number(res.headers.get("Retry-After"));
    return new AimultiError({
      status: res.status,
      code,
      message,
      method,
      url,
      requestId: res.headers.get("X-Request-Id") ?? undefined,
      retryAfter: Number.isFinite(retry) && retry > 0 ? retry : undefined,
    });
  }

  private async request<T>(method: string, path: string, init: RequestOptions = {}): Promise<T> {
    const url = this.url(path, init.query);
    const res = await this.fetchImpl(url, {
      method,
      headers: this.headers(init),
      body: init.body === undefined ? undefined : JSON.stringify(init.body),
    });
    if (!res.ok) throw await this.failure(res, method, url);
    if (res.status === 204) return undefined as T;
    const text = await res.text();
    if (text === "") return undefined as T;
    return JSON.parse(text) as T;
  }

  private async download(path: string, init: RequestOptions = {}): Promise<DownloadedFile> {
    const url = this.url(path, init.query);
    const res = await this.fetchImpl(url, { method: "GET", headers: this.headers(init) });
    if (!res.ok) throw await this.failure(res, "GET", url);
    const data = new Uint8Array(await res.arrayBuffer());
    return {
      filename: filenameFrom(res.headers.get("Content-Disposition")),
      contentType: res.headers.get("Content-Type") ?? "",
      data,
    };
  }

  // ---- identity and sessions ----

  /** The authenticated caller, its organization, plan and permissions. */
  me(): Promise<Me> {
    return this.request<Me>("GET", "/me");
  }

  /** Email a single use sign in link. Answers the same for an unknown address. */
  requestMagicLink(email: string, redirectAfter?: string): Promise<void> {
    return this.request<void>("POST", "/auth/magic-link", {
      body: { email, redirect_after: redirectAfter },
    });
  }

  /** Rotate a single use refresh token into a new session. */
  refreshSession(refreshToken: string): Promise<Session> {
    return this.request<Session>("POST", "/auth/refresh", { body: { refresh_token: refreshToken } });
  }

  /** End the session of the refresh token. Idempotent. */
  logout(refreshToken?: string): Promise<void> {
    return this.request<void>("POST", "/auth/logout", { body: { refresh_token: refreshToken } });
  }

  /** The Google or GitHub URL to send a browser to, with state and PKCE set. */
  oauthStart(provider: "google" | "github", redirectAfter?: string): Promise<OAuthStart> {
    return this.request<OAuthStart>("GET", `/auth/oauth/${encodeURIComponent(provider)}/start`, {
      query: { redirect_after: redirectAfter },
    });
  }

  // ---- inboxes ----

  /** Create an inbox, or get the existing one for the same client id. */
  createInbox(request: CreateInboxRequest = {}): Promise<Inbox> {
    return this.request<Inbox>("POST", "/inboxes", { body: request });
  }

  /** List inboxes, newest first. */
  listInboxes(options: ListOptions = {}): Promise<InboxList> {
    return this.request<InboxList>("GET", "/inboxes", { query: listQuery(options) });
  }

  getInbox(address: string): Promise<Inbox> {
    return this.request<Inbox>("GET", `/inboxes/${encodeURIComponent(address)}`);
  }

  updateInbox(address: string, request: UpdateInboxRequest): Promise<Inbox> {
    return this.request<Inbox>("PATCH", `/inboxes/${encodeURIComponent(address)}`, { body: request });
  }

  /** Delete an inbox and its mail. The address is quarantined for 90 days. */
  deleteInbox(address: string): Promise<void> {
    return this.request<void>("DELETE", `/inboxes/${encodeURIComponent(address)}`);
  }

  // ---- threads ----

  /** Threads of one inbox, or of the organization when the address is omitted. */
  listThreads(address?: string, options: ListOptions = {}): Promise<ThreadList> {
    const path = address ? `/inboxes/${encodeURIComponent(address)}/threads` : "/threads";
    return this.request<ThreadList>("GET", path, { query: listQuery(options) });
  }

  getThread(threadId: string, includeReview = false): Promise<Thread> {
    return this.request<Thread>("GET", `/threads/${encodeURIComponent(threadId)}`, {
      query: { include_review: includeReview || undefined },
    });
  }

  getInboxThread(address: string, threadId: string, includeReview = false): Promise<Thread> {
    return this.request<Thread>(
      "GET",
      `/inboxes/${encodeURIComponent(address)}/threads/${encodeURIComponent(threadId)}`,
      { query: { include_review: includeReview || undefined } },
    );
  }

  // ---- messages ----

  /** Message metadata of an inbox. Bodies are left out. */
  listMessages(address: string, options: ListOptions = {}): Promise<MessageList> {
    return this.request<MessageList>("GET", `/inboxes/${encodeURIComponent(address)}/messages`, {
      query: listQuery(options),
    });
  }

  /** One message with bodies, extracted text and the gatekeeper verdict. */
  getMessage(address: string, message: string, includeReview = false): Promise<Message> {
    return this.request<Message>(
      "GET",
      `/inboxes/${encodeURIComponent(address)}/messages/${encodeURIComponent(message)}`,
      { query: { include_review: includeReview || undefined } },
    );
  }

  updateMessageLabels(address: string, message: string, add: string[], remove: string[]): Promise<Message> {
    return this.request<Message>(
      "PATCH",
      `/inboxes/${encodeURIComponent(address)}/messages/${encodeURIComponent(message)}`,
      { body: { add_labels: add, remove_labels: remove } },
    );
  }

  /** The RFC 822 source of a message. */
  async rawMessage(address: string, message: string, includeReview = false): Promise<Uint8Array> {
    const file = await this.download(
      `/inboxes/${encodeURIComponent(address)}/messages/${encodeURIComponent(message)}/raw`,
      { query: { include_review: includeReview || undefined }, accept: "message/rfc822" },
    );
    return file.data;
  }

  getAttachment(address: string, message: string, attachmentId: string, includeReview = false): Promise<DownloadedFile> {
    return this.download(
      `/inboxes/${encodeURIComponent(address)}/messages/${encodeURIComponent(message)}/attachments/${encodeURIComponent(attachmentId)}`,
      { query: { include_review: includeReview || undefined } },
    );
  }

  /** Queue a new message in a new thread. An idempotency key makes a retry safe. */
  send(address: string, request: SendRequest, idempotencyKey?: string): Promise<SendResult> {
    return this.request<SendResult>("POST", `/inboxes/${encodeURIComponent(address)}/messages/send`, {
      body: request,
      idempotencyKey,
    });
  }

  /** Queue a reply in the thread of the message. */
  reply(address: string, message: string, request: ReplyRequest, idempotencyKey?: string): Promise<SendResult> {
    return this.request<SendResult>(
      "POST",
      `/inboxes/${encodeURIComponent(address)}/messages/${encodeURIComponent(message)}/reply`,
      { body: request, idempotencyKey },
    );
  }

  /** Release a message held for review. Needs message_release; audited. */
  releaseMessage(address: string, message: string): Promise<Message> {
    return this.request<Message>(
      "POST",
      `/inboxes/${encodeURIComponent(address)}/messages/${encodeURIComponent(message)}/release`,
    );
  }

  // ---- drafts ----

  createDraft(address: string, request: CreateDraftRequest): Promise<Draft> {
    return this.request<Draft>("POST", `/inboxes/${encodeURIComponent(address)}/drafts`, { body: request });
  }

  listDrafts(address: string, options: ListOptions = {}): Promise<DraftList> {
    return this.request<DraftList>("GET", `/inboxes/${encodeURIComponent(address)}/drafts`, {
      query: { limit: options.limit, page_token: options.page_token },
    });
  }

  getDraft(address: string, draftId: string): Promise<Draft> {
    return this.request<Draft>("GET", draftPath(address, draftId));
  }

  /** Replace the fields that are set. `send_at: null` cancels a scheduled send. */
  updateDraft(address: string, draftId: string, request: UpdateDraftRequest): Promise<Draft> {
    return this.request<Draft>("PATCH", draftPath(address, draftId), { body: request });
  }

  deleteDraft(address: string, draftId: string): Promise<void> {
    return this.request<void>("DELETE", draftPath(address, draftId));
  }

  sendDraft(address: string, draftId: string, idempotencyKey?: string): Promise<SendResult> {
    return this.request<SendResult>("POST", `${draftPath(address, draftId)}/send`, { idempotencyKey });
  }

  // ---- events ----

  /**
   * The organization's event stream. Iterate it with `for await`; leaving the
   * loop closes the connection.
   */
  async *events(options: EventOptions = {}): AsyncGenerator<Event> {
    const url = this.url("/events/stream", {
      types: options.types?.join(","),
      inbox: options.inbox,
      last_event_id: options.last_event_id,
    });
    const controller = new AbortController();
    const headers = this.headers({ accept: "text/event-stream" });
    if (options.last_event_id) headers.set("Last-Event-ID", String(options.last_event_id));
    const res = await this.fetchImpl(url, { method: "GET", headers, signal: controller.signal });
    if (!res.ok) throw await this.failure(res, "GET", url);
    if (!res.body) throw new AimultiError({ status: res.status, code: "internal_error", message: "the stream has no body", method: "GET", url });

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    try {
      for (;;) {
        const { value, done } = await reader.read();
        if (done) return;
        buffer += decoder.decode(value, { stream: true });
        buffer = buffer.replace(/\r\n/g, "\n");
        for (;;) {
          const end = buffer.indexOf("\n\n");
          if (end < 0) break;
          const block = buffer.slice(0, end);
          buffer = buffer.slice(end + 2);
          const event = parseEvent(block);
          if (event) yield event;
        }
      }
    } finally {
      controller.abort();
      void reader.cancel().catch(() => undefined);
    }
  }

  // ---- domains ----

  createDomain(domain: string): Promise<Domain> {
    return this.request<Domain>("POST", "/domains", { body: { domain } });
  }

  async listDomains(): Promise<Domain[]> {
    const out = await this.request<{ domains: Domain[] }>("GET", "/domains");
    return out.domains ?? [];
  }

  getDomain(domain: string): Promise<Domain> {
    return this.request<Domain>("GET", `/domains/${encodeURIComponent(domain)}`);
  }

  deleteDomain(domain: string): Promise<void> {
    return this.request<void>("DELETE", `/domains/${encodeURIComponent(domain)}`);
  }

  verifyDomain(domain: string): Promise<Domain> {
    return this.request<Domain>("POST", `/domains/${encodeURIComponent(domain)}/verify`);
  }

  // ---- webhooks ----

  createWebhook(request: CreateWebhookRequest): Promise<Webhook> {
    return this.request<Webhook>("POST", "/webhooks", { body: request });
  }

  async listWebhooks(): Promise<Webhook[]> {
    const out = await this.request<{ webhooks: Webhook[] }>("GET", "/webhooks");
    return out.webhooks ?? [];
  }

  deleteWebhook(webhookId: string): Promise<void> {
    return this.request<void>("DELETE", `/webhooks/${encodeURIComponent(webhookId)}`);
  }

  // ---- keys and audit ----

  /** Issue a key with at most the caller's permissions. The key is shown once. */
  createApiKey(request: CreateAPIKeyRequest = {}): Promise<APIKey> {
    return this.request<APIKey>("POST", "/api-keys", { body: request });
  }

  async listApiKeys(): Promise<APIKey[]> {
    const out = await this.request<{ api_keys: APIKey[] }>("GET", "/api-keys");
    return out.api_keys ?? [];
  }

  revokeApiKey(keyId: string): Promise<void> {
    return this.request<void>("DELETE", `/api-keys/${encodeURIComponent(keyId)}`);
  }

  async listAuditLogs(limit?: number): Promise<AuditEntry[]> {
    const out = await this.request<{ entries: AuditEntry[] }>("GET", "/audit-logs", { query: { limit } });
    return out.entries ?? [];
  }

  // ---- billing ----

  billing(): Promise<Billing> {
    return this.request<Billing>("GET", "/billing");
  }

  createCheckout(request: CheckoutRequest): Promise<BillingSession> {
    return this.request<BillingSession>("POST", "/billing/checkout", { body: request });
  }

  createPortal(returnUrl?: string): Promise<BillingSession> {
    return this.request<BillingSession>("POST", "/billing/portal", { body: { return_url: returnUrl } });
  }

  // ---- data rights ----

  /** The organization's own data. Secrets are never included. */
  exportOrganization(): Promise<OrgExport> {
    return this.request<OrgExport>("GET", "/export");
  }

  /** Erase the organization. `confirm` must equal the organization name. */
  deleteOrganization(confirm: string): Promise<DeleteOrganizationResult> {
    return this.request<DeleteOrganizationResult>("DELETE", "/organization", { body: { confirm } });
  }

  // ---- support queue ----

  listTickets(filter: TicketFilter = {}): Promise<TicketList> {
    return this.request<TicketList>("GET", "/tickets", {
      query: {
        state: filter.state,
        priority: filter.priority,
        assignee: filter.assignee,
        inbox: filter.inbox,
        q: filter.q,
        limit: filter.limit,
        page_token: filter.page_token,
      },
    });
  }

  createTicket(request: CreateTicketRequest): Promise<Ticket> {
    return this.request<Ticket>("POST", "/tickets", { body: request });
  }

  ticketStats(): Promise<TicketStats> {
    return this.request<TicketStats>("GET", "/tickets/stats");
  }

  getTicket(ticketId: string): Promise<TicketDetail> {
    return this.request<TicketDetail>("GET", `/tickets/${encodeURIComponent(ticketId)}`);
  }

  updateTicket(ticketId: string, request: UpdateTicketRequest): Promise<Ticket> {
    return this.request<Ticket>("PATCH", `/tickets/${encodeURIComponent(ticketId)}`, { body: request });
  }

  addTicketNote(ticketId: string, body: string): Promise<TicketNote> {
    return this.request<TicketNote>("POST", `/tickets/${encodeURIComponent(ticketId)}/notes`, { body: { body } });
  }

  replyTicket(ticketId: string, request: ReplyRequest, idempotencyKey?: string): Promise<SendResult> {
    return this.request<SendResult>("POST", `/tickets/${encodeURIComponent(ticketId)}/reply`, {
      body: request,
      idempotencyKey,
    });
  }
}

function draftPath(address: string, draftId: string): string {
  return `/inboxes/${encodeURIComponent(address)}/drafts/${encodeURIComponent(draftId)}`;
}

function listQuery(options: ListOptions): Query {
  return {
    limit: options.limit,
    page_token: options.page_token,
    label: options.label,
    include_review: options.include_review || undefined,
  };
}

/** Reads the filename parameter of a Content-Disposition header. */
function filenameFrom(disposition: string | null): string {
  if (!disposition) return "";
  const match = /filename="?([^";]+)"?/i.exec(disposition);
  return match?.[1] ?? "";
}

/** Parses one server sent event block, skipping comments and keep alives. */
function parseEvent(block: string): Event | null {
  let id = "";
  let name = "";
  const data: string[] = [];
  for (const line of block.split("\n")) {
    if (line === "" || line.startsWith(":")) continue;
    const colon = line.indexOf(":");
    const field = colon < 0 ? line : line.slice(0, colon);
    const value = colon < 0 ? "" : line.slice(colon + 1).replace(/^ /, "");
    if (field === "id") id = value;
    else if (field === "event") name = value;
    else if (field === "data") data.push(value);
  }
  if (id === "" && name === "" && data.length === 0) return null;
  const raw = data.join("\n");
  let envelope: EventEnvelope | null = null;
  try {
    envelope = raw === "" ? null : (JSON.parse(raw) as EventEnvelope);
  } catch {
    envelope = null;
  }
  return { id, name, envelope, data: raw };
}

/** The event types a webhook can subscribe to. */
export const EVENT_TYPES: readonly EventType[] = [
  "message.received",
  "message.received.spam",
  "message.sent",
  "message.failed",
  "message.bounced",
  "message.complained",
  "message.security.completed",
  "message.security.review",
  "domain.verified",
  "draft.created",
  "inbox.sending.paused",
  "inbox.sending.resumed",
  "organization.sending.paused",
  "organization.sending.resumed",
  "security.av_unavailable",
] as const;

