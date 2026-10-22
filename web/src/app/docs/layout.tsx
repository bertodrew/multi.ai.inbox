import type { ReactNode } from "react";
import { Disclosure } from "@/components/Disclosure";
import { DocsNav, type NavGroup } from "@/components/docs/DocsNav";
import { docGroups } from "@/content/docs";

const groups: NavGroup[] = docGroups.map((g) => ({
  title: g.title,
  pages: g.pages.map((p) => ({ slug: p.slug, title: p.title })),
}));

export default function DocsLayout({ children }: { children: ReactNode }) {
  return (
    <div className="mx-auto w-full max-w-6xl px-4 sm:px-6 lg:grid lg:grid-cols-[14rem_minmax(0,1fr)] lg:gap-10">
      <aside className="lg:sticky lg:top-16 lg:h-[calc(100dvh-4rem)] lg:overflow-y-auto lg:py-10">
        <div className="hidden lg:block">
          <DocsNav groups={groups} />
        </div>
        <Disclosure
          className="mt-4 rounded-lg border border-border lg:hidden"
          summaryClassName="flex min-h-11 items-center justify-between px-4 text-sm font-medium"
          summary={
            <>
              Documentation menu <span aria-hidden="true" className="transition-transform group-open:rotate-45">+</span>
            </>
          }
        >
          <div className="border-t border-border px-2 py-3">
            <DocsNav groups={groups} />
          </div>
        </Disclosure>
      </aside>
      <div className="min-w-0 py-10">{children}</div>
    </div>
  );
}
