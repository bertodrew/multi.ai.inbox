// The operations the console calls, as "METHOD /path" keys of openapi.yaml.
// The path parameter names match the spec, so a typo is a type error here and
// a renamed path is a build error (spec-check.ts).
export const CONSOLE_ENDPOINTS = [
  "GET /v0/auth/me",
  "GET /v0/inboxes",
  "POST /v0/inboxes",
  "DELETE /v0/inboxes/{inbox}",
  "GET /v0/inboxes/{inbox}",
  "PATCH /v0/inboxes/{inbox}",
  "GET /v0/inboxes/{inbox}/threads",
  "GET /v0/threads/{thread}",
  "GET /v0/inboxes/{inbox}/messages",
  "POST /v0/inboxes/{inbox}/messages/send",
  "PATCH /v0/inboxes/{inbox}/messages/{message}",
  "POST /v0/inboxes/{inbox}/messages/{message}/reply",
  "POST /v0/inboxes/{inbox}/messages/{message}/release",
  "GET /v0/api-keys",
  "POST /v0/api-keys",
  "DELETE /v0/api-keys/{key}",
  "GET /v0/webhooks",
  "POST /v0/webhooks",
  "DELETE /v0/webhooks/{webhook}",
  "GET /v0/domains",
  "POST /v0/domains",
  "POST /v0/domains/{domain}/verify",
  "GET /v0/audit-logs",
  "GET /v0/tickets",
  "POST /v0/tickets",
  "GET /v0/tickets/stats",
  "GET /v0/tickets/{ticket}",
  "PATCH /v0/tickets/{ticket}",
  "POST /v0/tickets/{ticket}/notes",
  "POST /v0/tickets/{ticket}/reply",
] as const;

export type Endpoint = (typeof CONSOLE_ENDPOINTS)[number];
export type Method<E extends Endpoint> = E extends `${infer M} ${string}` ? M : never;

type PathOf<E extends Endpoint> = E extends `${string} ${infer P}` ? P : never;
type ParamNames<P extends string> = P extends `${string}{${infer N}}${infer Rest}` ? N | ParamNames<Rest> : never;

/** Path parameters an endpoint needs, keyed by their name in the spec. */
export type PathParams<E extends Endpoint> = [ParamNames<PathOf<E>>] extends [never]
  ? Record<string, never>
  : Record<ParamNames<PathOf<E>>, string>;

/** Fills {name} placeholders with URL encoded values. */
export function buildPath<E extends Endpoint>(endpoint: E, params: PathParams<E>): string {
  const path = endpoint.slice(endpoint.indexOf(" ") + 1);
  const values: Record<string, string> = params;
  return path.replace(/\{(\w+)\}/g, (_, name: string) => {
    const v = values[name];
    if (v === undefined) throw new Error(`missing path parameter ${name} for ${endpoint}`);
    return encodeURIComponent(v);
  });
}
