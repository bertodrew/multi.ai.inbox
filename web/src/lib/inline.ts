export type InlineToken =
  | { kind: "text"; value: string }
  | { kind: "code"; value: string }
  | { kind: "bold"; value: string }
  | { kind: "link"; value: string; href: string };

const pattern = /`([^`]+)`|\*\*([^*]+)\*\*|\[([^\]]+)\]\(([^)\s]+)\)/g;

/** Splits mini markdown (code, bold, links) into tokens. */
export function tokenize(text: string): InlineToken[] {
  const out: InlineToken[] = [];
  let last = 0;
  for (const m of text.matchAll(pattern)) {
    const index = m.index;
    if (index > last) out.push({ kind: "text", value: text.slice(last, index) });
    if (m[1] !== undefined) out.push({ kind: "code", value: m[1] });
    else if (m[2] !== undefined) out.push({ kind: "bold", value: m[2] });
    else if (m[3] !== undefined && m[4] !== undefined) out.push({ kind: "link", value: m[3], href: m[4] });
    last = index + m[0].length;
  }
  if (last < text.length) out.push({ kind: "text", value: text.slice(last) });
  return out;
}

/** Plain text without markup, for meta descriptions. */
export function plain(text: string): string {
  return tokenize(text)
    .map((t) => t.value)
    .join("");
}
