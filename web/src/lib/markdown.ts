import type { Block, DocPage } from "@/content/types";
import { endpointsFor, schemasFor } from "./api";
import { tokenize } from "./inline";
import { SITE_URL } from "./site";
import type { ApiField, ApiParam } from "./openapi-types";

/** Internal docs links point to the Markdown twins, other links become absolute. */
function mdHref(href: string): string {
  if (!href.startsWith("/")) return href;
  const [path = "", hash] = href.split("#");
  const clean = path.replace(/\/$/, "");
  const target = clean.startsWith("/docs/") && !clean.includes(".") ? `${clean}.md` : path;
  return `${SITE_URL}${target}${hash ? `#${hash}` : ""}`;
}

function inline(text: string): string {
  return tokenize(text)
    .map((t) => {
      switch (t.kind) {
        case "code":
          return `\`${t.value}\``;
        case "bold":
          return `**${t.value}**`;
        case "link":
          return `[${t.value}](${mdHref(t.href)})`;
        default:
          return t.value;
      }
    })
    .join("");
}

const cell = (s: string): string => s.replace(/\|/g, "\\|").replace(/\n/g, " ");

function table(head: string[], rows: string[][]): string {
  return [
    `| ${head.map(cell).join(" | ")} |`,
    `| ${head.map(() => "---").join(" | ")} |`,
    ...rows.map((r) => `| ${r.map(cell).join(" | ")} |`),
  ].join("\n");
}

function fields(rows: Array<ApiField | ApiParam>): string {
  return table(
    ["Name", "Type", "Notes"],
    rows.map((r) => [
      `\`${r.name}\``,
      r.type,
      ["in" in r ? `in ${r.in}` : "", r.required ? "required" : "", r.description, ...r.constraints].filter(Boolean).join(", "),
    ]),
  );
}

function fence(lang: string, code: string): string {
  return "```" + lang + "\n" + code + "\n```";
}

function block(b: Block): string {
  switch (b.type) {
    case "p":
      return inline(b.text);
    case "h2":
      return `## ${inline(b.text)}`;
    case "h3":
      return `### ${inline(b.text)}`;
    case "list":
      return b.items.map((item, i) => `${b.ordered ? `${i + 1}.` : "*"} ${inline(item)}`).join("\n");
    case "code":
      return fence(b.sample.lang, b.sample.code);
    case "tabs":
      return b.samples.map((s) => `${s.label}:\n\n${fence(s.lang, s.code)}`).join("\n\n");
    case "table":
      return table(b.head.map(inline), b.rows.map((r) => r.map(inline)));
    case "note":
      return `> ${b.tone === "warn" ? "Careful" : "Note"}: ${inline(b.text)}`;
    case "endpoints":
      return endpointsFor(b.keys)
        .map((e) => {
          const parts = [`### ${e.method} ${e.path}`, e.summary];
          if (e.params.length > 0) parts.push(`Parameters:\n\n${fields(e.params)}`);
          if (e.body) parts.push(`Body${e.body.schema ? ` (${e.body.schema})` : ""}:\n\n${fields(e.body.fields)}`);
          parts.push(
            `Responses:\n\n${e.responses.map((r) => `* \`${r.code}\` ${r.description}${r.schema ? `: ${r.schema}` : ""}`).join("\n")}`,
          );
          return parts.join("\n\n");
        })
        .join("\n\n");
    case "schemas":
      return schemasFor(b.names)
        .map((s) => `### ${s.name} object\n\n${s.fields.length > 0 ? fields(s.fields) : "No documented fields."}`)
        .join("\n\n");
    default: {
      const exhaustive: never = b;
      return exhaustive;
    }
  }
}

export function docToMarkdown(page: DocPage): string {
  const body = page.blocks.map(block).join("\n\n");
  return `# ${page.title}\n\n> ${page.description}\n\nSource: ${SITE_URL}/docs/${page.slug}/\n\n${body}\n`;
}
