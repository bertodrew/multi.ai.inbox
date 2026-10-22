import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { docPages, getDoc } from "@/content/docs";
import { plain } from "@/lib/inline";
import { slugify } from "@/lib/slug";
import { Eyebrow } from "../ui";
import { DocBlocks } from "./DocBlocks";

export function docMetadata(slug: string): Metadata {
  const page = getDoc(slug);
  if (!page) return {};
  return {
    title: page.title,
    description: page.description,
    alternates: { types: { "text/markdown": `/docs/${slug}.md` } },
  };
}

export function DocArticle({ slug }: { slug: string }) {
  const page = getDoc(slug);
  if (!page) notFound();
  const index = docPages.findIndex((p) => p.slug === slug);
  const prev = index > 0 ? docPages[index - 1] : undefined;
  const next = index < docPages.length - 1 ? docPages[index + 1] : undefined;
  const toc = page.blocks.flatMap((b) => (b.type === "h2" ? [plain(b.text)] : []));

  return (
    <article className="max-w-3xl">
      <Eyebrow>Docs</Eyebrow>
      <h1 className="mt-2 font-display text-3xl font-semibold tracking-tight sm:text-4xl">{page.title}</h1>
      <p className="mt-3 text-lg text-muted">{page.description}</p>
      <p className="mt-3 text-sm">
        <a className="text-primary underline underline-offset-2" href={`/docs/${slug}.md`}>
          View as Markdown
        </a>
      </p>
      {toc.length > 2 ? (
        <nav aria-label="On this page" className="mt-6 rounded-lg border border-border bg-card px-4 py-3">
          <p className="font-mono text-xs tracking-[0.08em] text-muted uppercase">On this page</p>
          <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm">
            {toc.map((t) => (
              <li key={t}>
                <a className="text-primary hover:underline" href={`#${slugify(t)}`}>
                  {t}
                </a>
              </li>
            ))}
          </ul>
        </nav>
      ) : null}
      <div className="prose-doc mt-6">
        <DocBlocks blocks={page.blocks} />
      </div>
      <nav aria-label="Previous and next" className="mt-12 grid gap-3 border-t border-border pt-6 sm:grid-cols-2">
        {prev ? (
          <Link href={`/docs/${prev.slug}/`} className="rounded-lg border border-border p-4 hover:border-fg/40">
            <span className="block text-xs text-muted">Previous</span>
            <span className="font-medium">{prev.title}</span>
          </Link>
        ) : (
          <span />
        )}
        {next ? (
          <Link href={`/docs/${next.slug}/`} className="rounded-lg border border-border p-4 text-right hover:border-fg/40">
            <span className="block text-xs text-muted">Next</span>
            <span className="font-medium">{next.title}</span>
          </Link>
        ) : null}
      </nav>
    </article>
  );
}
