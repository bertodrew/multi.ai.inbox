import type { ReactNode } from "react";
import { slugify } from "@/lib/slug";
import { Container, DraftNotice, PageHeader } from "./ui";

export type LegalSection = { title: string; body: ReactNode };

export function LegalPage({
  eyebrow,
  title,
  updated,
  intro,
  sections,
}: {
  eyebrow: string;
  title: string;
  updated: string;
  intro?: ReactNode;
  sections: LegalSection[];
}) {
  return (
    <>
      <PageHeader eyebrow={eyebrow} title={title} intro={intro} />
      <Container className="py-10">
        <div className="max-w-3xl">
          <DraftNotice />
          <p className="mt-4 text-sm text-muted">
            Last updated: <time dateTime={updated}>{updated}</time>
          </p>
          <nav aria-label="Sections" className="mt-6 rounded-lg border border-border bg-card px-4 py-3">
            <ol className="list-decimal space-y-1 pl-5 text-sm">
              {sections.map((s) => (
                <li key={s.title}>
                  <a className="text-primary hover:underline" href={`#${slugify(s.title)}`}>
                    {s.title}
                  </a>
                </li>
              ))}
            </ol>
          </nav>
          <div className="prose-doc mt-8">
            {sections.map((s, i) => (
              <section key={s.title} aria-labelledby={slugify(s.title)}>
                <h2 id={slugify(s.title)}>
                  {i + 1}. {s.title}
                </h2>
                {s.body}
              </section>
            ))}
          </div>
        </div>
      </Container>
    </>
  );
}
