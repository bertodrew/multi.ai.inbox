import Link from "next/link";
import { tokenize } from "@/lib/inline";

export function Inline({ text }: { text: string }) {
  return (
    <>
      {tokenize(text).map((t, i) => {
        switch (t.kind) {
          case "code":
            return <code key={i}>{t.value}</code>;
          case "bold":
            return <strong key={i}>{t.value}</strong>;
          case "link":
            if (t.href.startsWith("/")) {
              return (
                <Link key={i} href={withSlash(t.href)}>
                  {t.value}
                </Link>
              );
            }
            return (
              <a key={i} href={t.href} rel={t.href.startsWith("http") ? "noreferrer" : undefined}>
                {t.value}
              </a>
            );
          default:
            return <span key={i}>{t.value}</span>;
        }
      })}
    </>
  );
}

/** Internal links end with a slash to match the static export layout. */
function withSlash(href: string): string {
  const [path = "", hash] = href.split("#");
  const fixed = path.endsWith("/") || path.includes(".") ? path : `${path}/`;
  return hash ? `${fixed}#${hash}` : fixed;
}
