import { authentication, quickstart } from "./getting-started";
import { attachments, inboxes, messagesAndThreads, sendingAndReplying } from "./resources";
import { apiReferencePage, customDomains, errors, limits, mcp, webhooks } from "./platform";
import { versions } from "./versions";
import type { DocPage } from "../types";

export type DocGroup = { title: string; pages: DocPage[] };

export const docGroups: DocGroup[] = [
  { title: "Get started", pages: [quickstart, authentication] },
  { title: "Mail", pages: [inboxes, messagesAndThreads, sendingAndReplying, attachments] },
  { title: "Platform", pages: [customDomains, webhooks, mcp] },
  { title: "Reference", pages: [errors, limits, versions, apiReferencePage] },
];

export const docPages: DocPage[] = docGroups.flatMap((g) => g.pages);

export function getDoc(slug: string): DocPage | undefined {
  return docPages.find((p) => p.slug === slug);
}
