"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

export type NavGroup = { title: string; pages: Array<{ slug: string; title: string }> };

export function DocsNav({ groups }: { groups: NavGroup[] }) {
  const pathname = usePathname();
  const current = pathname.replace(/\/$/, "");
  return (
    <nav aria-label="Documentation">
      <Link
        href="/docs/"
        aria-current={current === "/docs" ? "page" : undefined}
        className="block rounded-md px-2 py-1.5 text-sm hover:bg-card aria-[current=page]:bg-card aria-[current=page]:font-medium"
      >
        Overview
      </Link>
      {groups.map((g) => (
        <div key={g.title} className="mt-5">
          <p className="px-2 font-mono text-xs tracking-[0.08em] text-muted uppercase">{g.title}</p>
          <ul className="mt-1">
            {g.pages.map((p) => {
              const href = `/docs/${p.slug}`;
              return (
                <li key={p.slug}>
                  <Link
                    href={`${href}/`}
                    aria-current={current === href ? "page" : undefined}
                    className="block rounded-md px-2 py-1.5 text-sm hover:bg-card aria-[current=page]:bg-card aria-[current=page]:font-medium aria-[current=page]:text-primary"
                  >
                    {p.title}
                  </Link>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </nav>
  );
}
