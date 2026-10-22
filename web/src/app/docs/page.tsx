import type { Metadata } from "next";
import Link from "next/link";
import { docGroups } from "@/content/docs";
import { API_URL, MCP_URL } from "@/lib/site";
import { Eyebrow } from "@/components/ui";
import { slugify } from "@/lib/slug";

export const metadata: Metadata = {
  title: "Docs",
  description: "Documentation for the multi.ai.inbox REST API, webhooks and MCP server.",
};

export default function DocsIndex() {
  return (
    <div className="max-w-3xl">
      <Eyebrow>Docs</Eyebrow>
      <h1 className="mt-2 font-display text-3xl font-semibold tracking-tight sm:text-4xl">Documentation</h1>
      <p className="mt-3 text-lg text-muted">
        Email inboxes for AI agents over a REST API and an MCP server. Start with the quickstart, then read the
        page for the resource you need.
      </p>
      <dl className="mt-6 grid gap-3 rounded-xl border border-border bg-card p-4 text-sm sm:grid-cols-[auto_1fr] sm:gap-x-6">
        <dt className="font-mono text-xs tracking-[0.08em] text-muted uppercase sm:pt-0.5">REST base URL</dt>
        <dd className="font-mono break-all">{API_URL}/v0</dd>
        <dt className="font-mono text-xs tracking-[0.08em] text-muted uppercase sm:pt-0.5">MCP</dt>
        <dd className="font-mono break-all">{MCP_URL}</dd>
        <dt className="font-mono text-xs tracking-[0.08em] text-muted uppercase sm:pt-0.5">OpenAPI</dt>
        <dd className="font-mono break-all">{API_URL}/openapi.yaml</dd>
        <dt className="font-mono text-xs tracking-[0.08em] text-muted uppercase sm:pt-0.5">For agents</dt>
        <dd>
          <a className="text-primary underline underline-offset-2" href="/llms.txt">
            /llms.txt
          </a>{" "}
          indexes every page; each page has a Markdown twin.
        </dd>
      </dl>
      {docGroups.map((g) => (
        <section key={g.title} className="mt-10" aria-labelledby={`group-${slugify(g.title)}`}>
          <h2 id={`group-${slugify(g.title)}`} className="font-display text-xl font-semibold">
            {g.title}
          </h2>
          <ul className="mt-4 grid gap-3 sm:grid-cols-2">
            {g.pages.map((p) => (
              <li key={p.slug}>
                <Link
                  href={`/docs/${p.slug}/`}
                  className="block h-full rounded-xl border border-border bg-card p-4 hover:border-fg/40"
                >
                  <span className="font-medium">{p.title}</span>
                  <span className="mt-1 block text-sm text-muted">{p.description}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}
