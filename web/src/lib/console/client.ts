"use client";

// Fetch wrapper for the console. Every response body is parsed with a Zod
// schema; anything else (network failure, error envelope, unexpected shape)
// becomes an ApiError the caller shows to the user.
import type { z } from "zod";
import { buildPath, type Endpoint, type PathParams } from "./endpoints";
import {
  ErrorEnvelope,
  type CreateInboxBody,
  type CreateKeyBody,
  type CreateTicketBody,
  type CreateWebhookBody,
  type ReplyBody,
  type UpdateTicketBody,
  type SendBody,
} from "./schemas";
import type { ConsoleSession } from "./session";

/** Request bodies by endpoint, shaped after the request schemas in openapi.yaml. */
type Bodies = {
  "POST /v0/inboxes": CreateInboxBody;
  "POST /v0/inboxes/{inbox}/messages/send": SendBody;
  "PATCH /v0/inboxes/{inbox}/messages/{message}": { add_labels?: string[]; remove_labels?: string[] };
  "POST /v0/inboxes/{inbox}/messages/{message}/reply": ReplyBody;
  "POST /v0/api-keys": CreateKeyBody;
  "POST /v0/webhooks": CreateWebhookBody;
  "POST /v0/domains": { domain: string };
  "PATCH /v0/inboxes/{inbox}": { display_name?: string; ticket_intake?: boolean };
  "POST /v0/tickets": CreateTicketBody;
  "PATCH /v0/tickets/{ticket}": UpdateTicketBody;
  "POST /v0/tickets/{ticket}/notes": { body: string };
  "POST /v0/tickets/{ticket}/reply": ReplyBody;
};
type BodyOf<E extends Endpoint> = E extends keyof Bodies ? Bodies[E] : never;

type Query = Record<string, string | number | boolean | undefined>;

export type CallOptions<E extends Endpoint> = {
  params: PathParams<E>;
  query?: Query;
  body?: BodyOf<E>;
  idempotencyKey?: string;
  signal?: AbortSignal;
};

export type ApiErrorKind = "network" | "http" | "invalid_response" | "timeout";

export class ApiError extends Error {
  constructor(
    readonly kind: ApiErrorKind,
    message: string,
    readonly status: number = 0,
    readonly code: string = "",
    readonly requestId: string = "",
  ) {
    super(message);
    this.name = "ApiError";
  }

  /** The key is valid but lacks a permission for this action. */
  get isPermission(): boolean {
    return this.code === "missing_permission" || this.code === "permission_escalation";
  }

  /** The key itself is rejected: sign in again. */
  get isAuth(): boolean {
    return this.status === 401;
  }
}

const TIMEOUT_MS = 20_000;

async function send<E extends Endpoint>(session: ConsoleSession, endpoint: E, opts: CallOptions<E>): Promise<Response> {
  const method = endpoint.slice(0, endpoint.indexOf(" "));
  const url = new URL(session.apiBase + buildPath(endpoint, opts.params));
  for (const [k, v] of Object.entries(opts.query ?? {})) {
    if (v !== undefined && v !== "") url.searchParams.set(k, String(v));
  }
  const headers: Record<string, string> = { Authorization: `Bearer ${session.apiKey}`, Accept: "application/json" };
  if (opts.body !== undefined) headers["Content-Type"] = "application/json";
  if (opts.idempotencyKey) headers["Idempotency-Key"] = opts.idempotencyKey;

  const timeout = new AbortController();
  const timer = setTimeout(() => timeout.abort(), TIMEOUT_MS);
  const signal = opts.signal ? AbortSignal.any([opts.signal, timeout.signal]) : timeout.signal;
  let res: Response;
  try {
    res = await fetch(url, {
      method,
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      signal,
      credentials: "omit",
      cache: "no-store",
    });
  } catch (err) {
    if (opts.signal?.aborted) throw err; // the caller cancelled; not a user facing error
    if (timeout.signal.aborted) {
      throw new ApiError("timeout", `The API at ${session.apiBase} did not answer within ${TIMEOUT_MS / 1000} seconds.`);
    }
    throw new ApiError(
      "network",
      `Could not reach the API at ${session.apiBase}. Check your connection and the API base; the API must allow this site's origin (CORS).`,
    );
  } finally {
    clearTimeout(timer);
  }

  if (!res.ok) {
    const requestId = res.headers.get("X-Request-Id") ?? "";
    let body: unknown = null;
    try {
      body = await res.json();
    } catch {
      body = null;
    }
    const env = ErrorEnvelope.safeParse(body);
    if (env.success) throw new ApiError("http", env.data.message, res.status, env.data.code, requestId);
    throw new ApiError("http", `The API answered ${res.status} ${res.statusText}.`.trim(), res.status, "", requestId);
  }
  return res;
}

/** Calls an endpoint and returns its JSON body parsed by `schema`. */
export async function call<E extends Endpoint, S extends z.ZodType>(
  session: ConsoleSession,
  endpoint: E,
  opts: CallOptions<E>,
  schema: S,
): Promise<z.output<S>> {
  const res = await send(session, endpoint, opts);
  let json: unknown;
  try {
    json = await res.json();
  } catch {
    throw new ApiError("invalid_response", `The API sent a response that is not JSON for ${endpoint}.`, res.status);
  }
  const parsed = schema.safeParse(json);
  if (!parsed.success) {
    const first = parsed.error.issues[0];
    const where = first ? `${first.path.join(".") || "body"}: ${first.message}` : "unknown field";
    throw new ApiError(
      "invalid_response",
      `The API response for ${endpoint} has an unexpected shape (${where}).`,
      res.status,
      "",
      res.headers.get("X-Request-Id") ?? "",
    );
  }
  return parsed.data;
}

/** Calls an endpoint that answers 204 No Content. */
export async function callNoContent<E extends Endpoint>(
  session: ConsoleSession,
  endpoint: E,
  opts: CallOptions<E>,
): Promise<void> {
  await send(session, endpoint, opts);
}

/** A readable message for any thrown value. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.requestId ? `${err.message} (request ${err.requestId})` : err.message;
  if (err instanceof Error) return err.message;
  return "Something went wrong.";
}

export function isAbort(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}
