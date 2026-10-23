// Types written by hand from openapi/openapi.yaml. Field names are the wire
// format, so a value can be passed straight back to the API without a mapping
// layer. Where the spec allows a missing value the field is optional.

export type Plan = "free" | "builder" | "growth" | "team";
export type Role = "owner" | "admin" | "member";
export type EventType =
  | "message.received"
  | "message.received.spam"
  | "message.sent"
  | "message.failed"
  | "message.bounced"
  | "message.complained"
  | "message.security.completed"
  | "message.security.review"
  | "domain.verified"
  | "draft.created"
  | "inbox.sending.paused"
  | "inbox.sending.resumed"
  | "organization.sending.paused"
  | "organization.sending.resumed"
  | "security.av_unavailable";
export type Permission =
  | "inbox_read"
  | "inbox_write"
  | "message_read"
  | "message_send"
  | "message_update"
  | "domain_read"
  | "domain_write"
  | "webhook_read"
  | "webhook_write"
  | "api_key_read"
  | "api_key_write"
  | "audit_read"
  | "message_review"
  | "message_release"
  | "billing_read"
  | "billing_write"
  | "data_export"
  | "data_delete"
  | "ticket_read"
  | "ticket_write";
export type TicketState = "new" | "open" | "pending" | "resolved" | "closed";
export type TicketPriority = "low" | "normal" | "high" | "urgent";

/** The API error envelope, as it arrives on a 4xx or 5xx response. */
export interface ErrorEnvelope {
  name: string;
  code: string;
  message: string;
}

export interface MeUser {
  user_id: string;
  email: string;
}

export interface Org {
  organization_id: string;
  name: string;
  plan: Plan;
  screen_before_notify: boolean;
}

export interface Me {
  auth: "api_key" | "session";
  organization_id: string;
  name: string;
  plan: Plan;
  screen_before_notify: boolean;
  role?: Role;
  api_key_id?: string;
  user?: MeUser;
  organization?: Org;
  permissions: string[];
  expires_at: string;
}

export interface User {
  user_id: string;
  email: string;
  name: string;
  provider: "magic_link" | "google" | "github";
  role: Role;
  created_at: string;
  last_login_at: string;
}

export interface Session {
  access_token: string;
  refresh_token: string;
  token_type: "Bearer";
  expires_in: number;
  expires_at: string;
  refresh_expires_at: string;
  role: Role;
  user: User;
  organization: Org;
}

export interface OAuthStart {
  provider: "google" | "github";
  authorize_url: string;
  state: string;
  expires_at: string;
}

export interface CreateInboxRequest {
  username?: string;
  domain?: string;
  display_name?: string;
  client_id?: string;
}

export interface UpdateInboxRequest {
  display_name?: string;
  ticket_intake?: boolean;
}

export interface Inbox {
  id: string;
  inbox_id: string;
  email: string;
  display_name: string;
  client_id?: string;
  ticket_intake: boolean;
  sending_paused_at?: string;
  sending_paused_reason?: string;
  created_at: string;
  updated_at: string;
}

export interface InboxList {
  count: number;
  next_page_token?: string;
  inboxes: Inbox[];
}

export interface Message {
  id: string;
  message_id: string;
  thread_id: string;
  inbox_id: string;
  direction: "in" | "out";
  from: string;
  to: string[];
  cc?: string[];
  subject: string;
  text?: string;
  html?: string;
  extracted_text?: string;
  preview?: string;
  labels?: string[];
  in_reply_to?: string;
  references?: string[];
  status?: "received" | "queued" | "sending" | "sent" | "failed";
  authentication_results?: {
    dkim?: string;
    spf?: string;
    dmarc?: string;
    dmarc_policy?: string;
    antivirus?: "clean" | "unscanned";
    spam_score?: number;
    spam_action?: string;
    internal?: boolean;
  };
  gatekeeper?: {
    status?: "pending" | "complete" | "unavailable" | "timeout";
    verdict?: "clear" | "review";
    score?: number;
    reasons?: string[];
    model?: string;
  };
  screening?: "review" | "allowed" | "released";
  attachments?: AttachmentMeta[];
  created_at: string;
}

export interface AttachmentMeta {
  attachment_id: string;
  filename: string;
  content_type: string;
  size: number;
}

export interface Thread {
  thread_id: string;
  inbox_id: string;
  subject: string;
  preview: string;
  labels: string[];
  message_count: number;
  timestamp: string;
  messages?: Message[];
}

export interface ThreadList {
  count: number;
  next_page_token?: string;
  threads: Thread[];
}

export interface MessageList {
  count: number;
  next_page_token?: string;
  messages: Message[];
}

/** An attachment to send. `content` is base64 on the wire. */
export interface Attachment {
  filename?: string;
  content_type?: string;
  content: string;
}

export interface SendRequest {
  to: string[];
  cc?: string[];
  bcc?: string[];
  subject?: string;
  text?: string;
  html?: string;
  labels?: string[];
  attachments?: Attachment[];
}

export interface ReplyRequest {
  text?: string;
  html?: string;
  reply_all?: boolean;
  labels?: string[];
  attachments?: Attachment[];
}

export interface SendResult {
  message_id: string;
  thread_id: string;
}

export interface CreateDraftRequest {
  to?: string[];
  cc?: string[];
  bcc?: string[];
  subject?: string;
  text?: string;
  html?: string;
  labels?: string[];
  attachments?: Attachment[];
  in_reply_to?: string;
  reply_all?: boolean;
  send_at?: string;
}

/** `send_at: null` cancels a scheduled send, which is why it is not optional. */
export interface UpdateDraftRequest {
  to?: string[];
  cc?: string[];
  bcc?: string[];
  subject?: string;
  text?: string;
  html?: string;
  labels?: string[];
  attachments?: Attachment[];
  send_at?: string | null;
}

export interface Draft {
  draft_id: string;
  inbox_id: string;
  thread_id?: string;
  in_reply_to?: string;
  references?: string[];
  to?: string[];
  cc?: string[];
  bcc?: string[];
  subject?: string;
  text?: string;
  html?: string;
  labels?: string[];
  attachments?: Array<{ filename: string; content_type: string; size: number }>;
  send_at?: string;
  status?: "draft" | "scheduled" | "failed";
  last_error?: string;
  created_at: string;
  updated_at: string;
}

export interface DraftList {
  count: number;
  next_page_token?: string;
  drafts: Draft[];
}

export interface DNSRecord {
  type: "MX" | "TXT";
  name: string;
  value: string;
  status: "verified" | "missing";
}

export interface Domain {
  domain_id: string;
  domain: string;
  shared: boolean;
  status: "pending" | "verified";
  records: DNSRecord[];
  last_checked_at?: string;
  verified_at?: string;
}

export interface Webhook {
  webhook_id: string;
  url: string;
  event_types: EventType[];
  inbox_ids?: string[];
  enabled: boolean;
  /** Returned only on create. */
  secret?: string;
}

export interface CreateWebhookRequest {
  url: string;
  event_types: EventType[];
  inbox_ids?: string[];
}

export interface APIKey {
  api_key_id: string;
  /** Returned only on create. */
  api_key?: string;
  prefix: string;
  name: string;
  permissions: Permission[] | null;
  expires_at?: string;
  last_used_at?: string;
  created_at: string;
}

export interface CreateAPIKeyRequest {
  name?: string;
  permissions?: Permission[];
  expires_at?: string;
}

export interface AuditEntry {
  id: number;
  actor: string;
  action: string;
  target: string;
  ip: string;
  request_id: string;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface Billing {
  plan: Plan;
  status:
    | "none"
    | "incomplete"
    | "incomplete_expired"
    | "trialing"
    | "active"
    | "past_due"
    | "unpaid"
    | "canceled"
    | "paused";
  interval?: "monthly" | "yearly";
  current_period_end?: string;
  cancel_at_period_end: boolean;
  past_due_since?: string;
  grace_period_ends_at?: string;
}

export interface CheckoutRequest {
  plan: "builder" | "growth" | "team";
  interval?: "monthly" | "yearly";
  success_url?: string;
  cancel_url?: string;
}

export interface BillingSession {
  session_id: string;
  url: string;
}

export interface OrgExport {
  format: string;
  generated_at: string;
  organization: Org;
  inboxes: Array<{
    address: string;
    display_name: string;
    client_id: string;
    created_at: string;
    messages: number;
  }>;
  domains: Array<{ domain: string; status: "verified" | "pending"; created_at: string; verified_at: string }>;
  webhooks: Array<{
    webhook_id: string;
    url: string;
    event_types: EventType[];
    enabled: boolean;
    created_at: string;
  }>;
  api_keys: Array<{
    api_key_id: string;
    name: string;
    permissions: Permission[];
    created_at: string;
    expires_at?: string;
    last_used_at?: string;
    revoked_at?: string;
  }>;
  counts: Record<string, number>;
  audit_log: AuditEntry[];
  notice: string;
  /** Ticket records are passed through as they are, because their shape grows. */
  tickets?: unknown;
  ticket_notes?: unknown;
  ticket_events?: unknown;
}

export interface DeletionResult {
  organization_id: string;
  deleted: boolean;
  blobs_queued: number;
  addresses_quarantined: number;
  custom_domains_released: string[];
}

export interface DeletionSummary {
  organization: Org;
  created_at: string;
  exported_at: string;
  inboxes: string[];
  custom_domains: string[];
  counts: Record<string, number>;
  api_keys_revoked: number;
  audit_log: AuditEntry[];
}

export interface DeleteOrganizationResult {
  deletion: DeletionResult;
  export_before_deletion: DeletionSummary;
}

export interface Ticket {
  id: string;
  number: number;
  inbox_id: string;
  thread_id: string;
  subject: string;
  requester: string;
  state: TicketState;
  priority: TicketPriority;
  assignee_user?: string;
  assignee_key?: string;
  sla_policy?: string;
  due_first_reply?: string;
  due_resolution?: string;
  first_reply_at?: string;
  resolved_at?: string;
  closed_at?: string;
  tags?: string[];
  created_at: string;
  updated_at: string;
}

export interface TicketList {
  count: number;
  next_page_token?: string;
  tickets: Ticket[];
}

export interface TicketEvent {
  id: number;
  kind: string;
  actor: string;
  detail?: Record<string, unknown>;
  created_at: string;
}

export interface TicketNote {
  id: string;
  author: string;
  body: string;
  created_at: string;
}

export interface TicketDetail {
  ticket: Ticket;
  events: TicketEvent[];
  notes: TicketNote[];
}

export interface TicketStats {
  by_state: Record<string, number>;
  open: number;
  breaching: number;
  unassigned: number;
  median_first_reply_seconds: number;
}

export interface CreateTicketRequest {
  thread_id: string;
  priority?: TicketPriority;
  tags?: string[];
  assignee_user?: string;
  assignee_key?: string;
}

export interface UpdateTicketRequest {
  state?: TicketState;
  priority?: TicketPriority;
  tags?: string[];
  assignee_user?: string;
  assignee_key?: string;
}

/** The payload of a webhook and of a streamed event. */
export interface EventEnvelope<T = unknown> {
  type: string;
  event_type: EventType;
  event_id: string;
  created_at: string;
  data: T;
}

/** One server sent event from the stream. */
export interface Event<T = unknown> {
  /** Position in the stream, for resuming. */
  id: string;
  /** The event type. */
  name: string;
  /** The parsed envelope. */
  envelope: EventEnvelope<T> | null;
  /** The raw data line, when it did not parse as an envelope. */
  data: string;
}

export interface ListOptions {
  /** Page size, 1 to 100. The API default is 20. */
  limit?: number;
  page_token?: string;
  /** Filter messages and threads by label. */
  label?: string;
  /** Also return messages held for review. Needs message_review. */
  include_review?: boolean;
}

export interface TicketFilter {
  state?: TicketState;
  priority?: TicketPriority;
  assignee?: string;
  inbox?: string;
  q?: string;
  limit?: number;
  page_token?: string;
}

export interface EventOptions {
  /** Event types to deliver. All when empty. */
  types?: EventType[];
  /** Only events of this inbox. */
  inbox?: string;
  /** Resume after this event id, for a reconnect. */
  last_event_id?: number;
}
