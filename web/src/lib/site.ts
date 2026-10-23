// Single source of truth for URLs, addresses and legal placeholders.
// The brand domain is not bought yet: links inside the site stay relative,
// and absolute URLs (llms.txt, metadata) use SITE_URL.

export const SITE_NAME = "multi.ai.inbox";

/** Public URL of this website. Change when the brand domain is live. */
export const SITE_URL = "https://aimulti.inbox";

/** Base URL of the REST API and MCP endpoint (staging until launch). */
export const API_URL = "https://api.162-55-213-24.sslip.io";

/** MCP endpoint, same API key as REST. */
export const MCP_URL = `${API_URL}/mcp`;

export const CONTACT_EMAIL = "hello@aimulti.inbox";
export const SECURITY_EMAIL = "security@aimulti.inbox";
export const PRIVACY_EMAIL = "privacy@aimulti.inbox";
export const CAREERS_EMAIL = "jobs@aimulti.inbox";

/** Environment label shown in the footer. */
export const STATUS_NOTE = "Staging";

/** Controller details for the privacy policy and terms. Placeholders until incorporation. */
export const CONTROLLER = {
  name: "[Company legal name]",
  address: "[Registered address, EU]",
  register: "[Company register and number]",
  vat: "[VAT ID]",
  representative: "[Legal representative]",
} as const;

export const LEGAL_LAST_UPDATED = "2026-10-22";

/** Search engines may index the site. Keep false until the brand domain is live. */
export const INDEXABLE = false;
