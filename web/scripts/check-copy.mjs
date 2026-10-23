// Fails the build when visible text in the export uses dash characters as
// punctuation (em dash, en dash, or a hyphen between spaces). Code blocks are
// skipped for the spaced hyphen; em and en dashes are refused everywhere.
import { readdirSync, readFileSync, statSync, existsSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

// fileURLToPath decodes %20: the repo path contains a space ("4TB SSD") and
// URL.pathname leaves it percent-encoded, failing existsSync.
const root = fileURLToPath(new URL("../out/", import.meta.url));
if (!existsSync(join(root, "index.html"))) {
  console.error("check-copy: out/index.html is missing");
  process.exit(1);
}

function walk(dir) {
  return readdirSync(dir).flatMap((name) => {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) return name === "_next" ? [] : walk(p);
    return [p];
  });
}

const decode = (s) =>
  s
    .replace(/&nbsp;/g, " ")
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&#x27;|&#39;/g, "'")
    .replace(/&#(\d+);/g, (_, n) => String.fromCodePoint(Number(n)))
    .replace(/&#x([0-9a-f]+);/gi, (_, n) => String.fromCodePoint(parseInt(n, 16)));

function visibleText(html) {
  return decode(
    html
      .replace(/<script[\s\S]*?<\/script>/gi, " ")
      .replace(/<style[\s\S]*?<\/style>/gi, " ")
      .replace(/<pre[\s\S]*?<\/pre>/gi, " ")
      .replace(/<code[\s\S]*?<\/code>/gi, " ")
      .replace(/<[^>]+>/g, " "),
  );
}

const problems = [];
for (const file of walk(root)) {
  const rel = relative(root, file);
  if (!/\.(html|md|txt)$/.test(rel) || /(^|\/)(__next|index\.txt$)/.test(rel)) continue;
  const raw = readFileSync(file, "utf8");
  const text = rel.endsWith(".html") ? visibleText(raw) : raw.replace(/```[\s\S]*?```/g, " ").replace(/`[^`]*`/g, " ");
  const hits = [];
  for (const [label, re] of [
    ["em dash", /—/g],
    ["en dash", /–/g],
    ["spaced hyphen", / - /g],
  ]) {
    for (const m of text.matchAll(re)) {
      hits.push(`${label}: "...${text.slice(Math.max(0, m.index - 30), m.index + 30).replace(/\s+/g, " ")}..."`);
    }
  }
  if (/[–—]/.test(raw) && !hits.some((h) => h.startsWith("em") || h.startsWith("en"))) {
    hits.push("em or en dash inside code");
  }
  if (hits.length) problems.push(`${rel}\n  ${hits.join("\n  ")}`);
}

if (problems.length) {
  console.error(`check-copy: dash punctuation found\n${problems.join("\n")}`);
  process.exit(1);
}
console.log("check-copy: no dash punctuation in visible text");
