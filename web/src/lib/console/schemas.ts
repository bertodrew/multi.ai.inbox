// Zod schemas for the console: every response the console reads and every
// request body it sends. Written by hand from openapi/openapi.yaml and the
// JSON the Go handlers emit (internal/core). Go encodes nil slices as null,
// so list fields accept null and normalise it to an empty array.
import { z } from "zod";

/** A list field that Go may send as null. */
const list = <T extends z.ZodType>(item: T) =>
  z
    .array(item)
    .nullish()
    .transform((v) => v ?? []);

const timestamp = z.iso.datetime({ offset: true });

// Enums from components.schemas in openapi.yaml. specDrift() in spec-check.ts
// fails the build when these and the spec disagree.
export const PERMISSIONS = [
  "inbox_read",
  "inbox_write",
  "message_read",
  "message_send",
  "message_update",
  "domain_read",
  "domain_write",
  "webhook_read",
  "webhook_write",
  "api_key_read",
  "api_key_write",
  "audit_read",
  "message_review",
  "message_release",
  "billing_read",
  "billing_write",
] as const;
export const Permission = z.enum(PERMISSIONS);
export type Permission = z.infer<typeof Permission>;

export const EVENT_TYPES = [
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
export const EventType = z.enum(EVENT_TYPES);
export type EventType = z.infer<typeof EventType>;

export const ErrorEnvelope = z.object({
  name: z.string(),
  code: z.string(),
  message: z.string(),
});

export const Me = z.object({
  organization_id: z.string(),
  name: z.string(),
  plan: z.string(),
  screen_before_notify: z.boolean(),
  api_key_id: z.string().nullish(),
  /** Every permission the key holds; the API expands a full access key. */
  permissions: z.array(z.string()),
  expires_at: timestamp.nullish(),
});
export type Me = z.infer<typeof Me>;

export const Inbox = z.object({
  id: z.string(),
  inbox_id: z.string(),
  email: z.string(),
  display_name: z.string(),
  client_id: z.string().nullish(),
  sending_paused_at: timestamp.nullish(),
  sending_paused_reason: z.string().nullish(),
  created_at: timestamp,
  updated_at: timestamp,
});
export type Inbox = z.infer<typeof Inbox>;

export const InboxList = z.object({
  count: z.number().int(),
  next_page_token: z.string().optional(),
  inboxes: list(Inbox),
});

const Gatekeeper = z.object({
  status: z.string().optional(),
  verdict: z.string().optional(),
  score: z.number().optional(),
  reasons: list(z.string()),
  model: z.string().optional(),
});

const AttachmentInfo = z.object({
  attachment_id: z.string(),
  filename: z.string(),
  content_type: z.string(),
  size: z.number().int(),
});

export const Message = z.object({
  id: z.string(),
  message_id: z.string(),
  thread_id: z.string(),
  inbox_id: z.string(),
  direction: z.enum(["in", "out"]),
  from: z.string(),
  to: list(z.string()),
  cc: list(z.string()),
  bcc: list(z.string()),
  subject: z.string(),
  text: z.string().nullish(),
  html: z.string().nullish(),
  extracted_text: z.string().nullish(),
  preview: z.string(),
  labels: list(z.string()),
  in_reply_to: z.string().nullish(),
  references: list(z.string()),
  status: z.enum(["received", "queued", "sending", "sent", "failed"]),
  authentication_results: z.record(z.string(), z.unknown()).nullish(),
  gatekeeper: Gatekeeper.nullish(),
  screening: z.enum(["pending", "review", "allowed", "released"]).nullish(),
  attachments: list(AttachmentInfo),
  created_at: timestamp,
});
export type Message = z.infer<typeof Message>;

export const MessageList = z.object({
  count: z.number().int(),
  next_page_token: z.string().optional(),
  messages: list(Message),
});

export const Thread = z.object({
  thread_id: z.string(),
  inbox_id: z.string(),
  subject: z.string(),
  preview: z.string(),
  labels: list(z.string()),
  message_count: z.number().int(),
  timestamp: timestamp,
  messages: list(Message),
});
export type Thread = z.infer<typeof Thread>;

export const ThreadList = z.object({
  count: z.number().int(),
  next_page_token: z.string().optional(),
  threads: list(Thread),
});

export const SendResult = z.object({
  message_id: z.string(),
  thread_id: z.string(),
});

export const ApiKey = z.object({
  api_key_id: z.string(),
  api_key: z.string().optional(),
  prefix: z.string(),
  name: z.string(),
  /** null means every permission (a full access key). */
  permissions: z.array(z.string()).nullable(),
  expires_at: timestamp.nullish(),
  last_used_at: timestamp.nullish(),
  created_at: timestamp,
});
export type ApiKey = z.infer<typeof ApiKey>;

export const ApiKeyList = z.object({ count: z.number().int(), api_keys: list(ApiKey) });

export const AuditEntry = z.object({
  id: z.number().int(),
  actor: z.string(),
  action: z.string(),
  target: z.string(),
  ip: z.string(),
  request_id: z.string(),
  metadata: z.unknown(),
  created_at: timestamp,
});
export type AuditEntry = z.infer<typeof AuditEntry>;

export const AuditList = z.object({ count: z.number().int(), entries: list(AuditEntry) });

export const Webhook = z.object({
  webhook_id: z.string(),
  url: z.string(),
  event_types: list(z.string()),
  inbox_ids: list(z.string()),
  enabled: z.boolean(),
  secret: z.string().optional(),
  created_at: timestamp,
});
export type Webhook = z.infer<typeof Webhook>;

export const WebhookList = z.object({ count: z.number().int(), webhooks: list(Webhook) });

export const DnsRecord = z.object({
  type: z.string(),
  name: z.string(),
  value: z.string(),
  status: z.enum(["verified", "missing"]).optional(),
});

export const Domain = z.object({
  domain_id: z.string(),
  domain: z.string(),
  shared: z.boolean(),
  status: z.enum(["pending", "verified"]),
  records: list(DnsRecord),
  last_checked_at: timestamp.nullish(),
  verified_at: timestamp.nullish(),
  created_at: timestamp,
});
export type Domain = z.infer<typeof Domain>;

export const DomainList = z.object({ count: z.number().int(), domains: list(Domain) });

// Inputs. Constraints mirror openapi.yaml so the user sees the problem before
// the request leaves the browser; the API validates again.

const email = z.email("Enter a valid email address");

/** Splits a comma or newline separated field into trimmed addresses. */
export const addressList = (label: string, min: number) =>
  z
    .string()
    .transform((s) =>
      s
        .split(/[,\n]/)
        .map((p) => p.trim())
        .filter((p) => p !== ""),
    )
    .pipe(z.array(email).min(min, min > 0 ? `${label}: add at least one address` : undefined));

export const SignInInput = z.object({
  apiKey: z
    .string()
    .trim()
    .regex(/^mai_live_[0-9A-Za-z]{32}$/, "An API key is mai_live_ followed by 32 letters and digits"),
  apiBase: z
    .url({ protocol: /^https?$/, error: "The API base must be an http or https URL" })
    .transform((u) => u.replace(/\/+$/, "")),
});

export const CreateInboxInput = z.object({
  username: z
    .string()
    .trim()
    .toLowerCase()
    .regex(/^([a-z0-9]([a-z0-9._-]{0,62}[a-z0-9])?)?$/, "Use lowercase letters, digits, dots, underscores or hyphens")
    .optional(),
  domain: z.string().trim().toLowerCase().optional(),
  display_name: z.string().trim().max(200, "The display name is longer than 200 characters").optional(),
});
export type CreateInboxBody = z.infer<typeof CreateInboxInput>;

export const SendInput = z
  .object({
    to: addressList("To", 1),
    cc: addressList("Cc", 0),
    bcc: addressList("Bcc", 0),
    subject: z.string().trim().max(998, "The subject is too long"),
    text: z.string(),
  })
  .refine((v) => v.text.trim() !== "" || v.subject !== "", { message: "Write a subject or a message" });
export type SendBody = z.infer<typeof SendInput>;

export const ReplyInput = z.object({
  text: z.string().refine((t) => t.trim() !== "", "Write a reply first"),
  reply_all: z.boolean(),
});
export type ReplyBody = z.infer<typeof ReplyInput>;

/** Labels the platform manages (internal/core/core.go); the API refuses to change them. */
export const SYSTEM_LABELS: readonly string[] = ["received", "sent", "queued", "failed", "bounced"];

export const LabelInput = z
  .string()
  .trim()
  .toLowerCase()
  .min(1, "Type a label first")
  .max(64, "A label has at most 64 characters")
  .refine((l) => !SYSTEM_LABELS.includes(l), "This label is managed by the platform");

/** A label to filter by: empty clears the filter. */
export const LabelFilter = z
  .string()
  .trim()
  .toLowerCase()
  .max(64, "A label has at most 64 characters")
  .regex(/^\S*$/, "A label has no spaces");

export const CreateKeyInput = z.object({
  name: z.string().trim().min(1, "Give the key a name").max(100, "The name is longer than 100 characters"),
  permissions: z.array(Permission).min(1, "Pick at least one permission"),
  expires_at: z
    .string()
    .transform((s, ctx) => {
      if (s === "") return undefined;
      const d = new Date(s);
      if (Number.isNaN(d.getTime()) || d.getTime() <= Date.now()) {
        ctx.addIssue({ code: "custom", message: "The expiry must be a date in the future" });
        return z.NEVER;
      }
      return d.toISOString();
    })
    .optional(),
});
export type CreateKeyBody = z.infer<typeof CreateKeyInput>;

export const CreateWebhookInput = z.object({
  url: z.url({ protocol: /^https$/, error: "The webhook URL must start with https://" }),
  event_types: z.array(EventType).min(1, "Pick at least one event"),
  inbox_ids: addressList("Inboxes", 0).pipe(z.array(z.string()).max(10, "At most 10 inboxes")),
});
export type CreateWebhookBody = z.infer<typeof CreateWebhookInput>;

export const DomainInput = z
  .string()
  .trim()
  .toLowerCase()
  .regex(/^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/, "Enter a domain such as mail.example.com");
