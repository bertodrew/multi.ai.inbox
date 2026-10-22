import type { Metadata } from "next";
import { Container, PageHeader } from "@/components/ui";
import { CAREERS_EMAIL } from "@/lib/site";

export const metadata: Metadata = {
  title: "Careers",
  description: "multi.ai.inbox is built by a solo founder. Not hiring now, open applications welcome.",
};

const values = [
  { title: "Correct before clever", body: "Email has thirty years of edge cases. We read the RFCs and write the test first." },
  { title: "Boring infrastructure", body: "Postgres, Go, Rust and a few well understood tools, run on servers we can reason about." },
  { title: "Honest copy", body: "We state what exists today. Plans are labeled as plans, and nothing is claimed before it is true." },
  { title: "Privacy as a default", body: "EU hosting on every plan, no tracking on our own site, the least data we can keep." },
];

export default function CareersPage() {
  return (
    <>
      <PageHeader
        eyebrow="Careers"
        title="Not hiring yet"
        intro="multi.ai.inbox is built by a solo founder. There are no open roles today."
      />
      <Container className="py-12">
        <div className="max-w-3xl">
          <h2 className="font-display text-2xl font-semibold">Open applications</h2>
          <p className="mt-3">
            If you care about email infrastructure, agents or privacy and want to be considered when we do hire, write
            to{" "}
            <a className="text-primary underline underline-offset-2" href={`mailto:${CAREERS_EMAIL}`}>
              {CAREERS_EMAIL}
            </a>
            . Tell us what you have built and link to code or writing we can read. We reply to every message, even if
            the answer is not now.
          </p>
          <h2 className="mt-12 font-display text-2xl font-semibold">What we value</h2>
          <ul className="mt-6 grid gap-4 sm:grid-cols-2">
            {values.map((v) => (
              <li key={v.title} className="rounded-xl border border-border bg-card p-5">
                <h3 className="font-display font-semibold">{v.title}</h3>
                <p className="mt-2 text-sm text-muted">{v.body}</p>
              </li>
            ))}
          </ul>
        </div>
      </Container>
    </>
  );
}
