// Generates src/generated/openapi.ts from ../openapi/openapi.yaml.
// The YAML reader below supports the subset the spec uses: block mappings,
// block sequences, flow mappings and sequences (also across lines), quoted
// and plain scalars, and literal block scalars. No dependency needed.
// If the spec is not reachable (for example a build with only web/ as
// context), the committed generated file is kept as is.
import { existsSync, readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const specPath = resolve(here, "../../openapi/openapi.yaml");
const outPath = resolve(here, "../src/generated/openapi.ts");

if (!existsSync(specPath)) {
  if (existsSync(outPath)) {
    console.log(`gen-openapi: ${specPath} not found, keeping ${outPath}`);
    process.exit(0);
  }
  console.error(`gen-openapi: ${specPath} not found and no generated file exists`);
  process.exit(1);
}

// ---------- tiny YAML reader ----------

function stripComment(line) {
  let quote = "";
  for (let i = 0; i < line.length; i++) {
    const c = line[i];
    if (quote) {
      if (c === quote) quote = "";
      continue;
    }
    if (c === '"' || c === "'") quote = c;
    else if (c === "#" && (i === 0 || line[i - 1] === " ")) return line.slice(0, i).trimEnd();
  }
  return line.trimEnd();
}

function scalar(raw) {
  const s = raw.trim();
  if (s.startsWith('"') && s.endsWith('"')) return JSON.parse(s);
  if (s.startsWith("'") && s.endsWith("'")) return s.slice(1, -1).replace(/''/g, "'");
  if (s === "true") return true;
  if (s === "false") return false;
  if (s === "null" || s === "~" || s === "") return null;
  if (/^-?\d+(\.\d+)?$/.test(s)) return Number(s);
  return s;
}

function parseFlow(text) {
  let i = 0;
  const ws = () => {
    while (i < text.length && /\s/.test(text[i])) i++;
  };
  const value = () => {
    ws();
    const c = text[i];
    if (c === "{") return map();
    if (c === "[") return seq();
    return plain(true);
  };
  const plain = (isValue) => {
    ws();
    const c = text[i];
    if (c === '"' || c === "'") {
      const start = i++;
      while (i < text.length && text[i] !== c) {
        if (text[i] === "\\" && c === '"') i++;
        i++;
      }
      i++;
      return scalar(text.slice(start, i));
    }
    const start = i;
    while (i < text.length) {
      const ch = text[i];
      if (ch === "," || ch === "}" || ch === "]") break;
      if (!isValue && ch === ":" && (text[i + 1] === " " || text[i + 1] === undefined)) break;
      i++;
    }
    return scalar(text.slice(start, i));
  };
  const map = () => {
    i++;
    const out = {};
    for (;;) {
      ws();
      if (text[i] === "}") {
        i++;
        return out;
      }
      const key = String(plain(false));
      ws();
      if (text[i] !== ":") throw new Error(`flow map: expected ':' at ${i} in ${text}`);
      i++;
      out[key] = value();
      ws();
      if (text[i] === ",") i++;
    }
  };
  const seq = () => {
    i++;
    const out = [];
    for (;;) {
      ws();
      if (text[i] === "]") {
        i++;
        return out;
      }
      out.push(value());
      ws();
      if (text[i] === ",") i++;
    }
  };
  const v = value();
  return v;
}

function balance(s) {
  let depth = 0;
  let quote = "";
  for (const c of s) {
    if (quote) {
      if (c === quote) quote = "";
      continue;
    }
    if (c === '"' || c === "'") quote = c;
    else if (c === "{" || c === "[") depth++;
    else if (c === "}" || c === "]") depth--;
  }
  return depth;
}

function parseYaml(src) {
  const lines = src
    .split("\n")
    .map((raw) => {
      const text = stripComment(raw);
      return { indent: text.length - text.trimStart().length, text: text.trim(), raw };
    });
  let pos = 0;
  const skipBlank = () => {
    while (pos < lines.length && lines[pos].text === "") pos++;
  };

  // Reads an inline value that may continue on following lines (flow) or be a block scalar.
  const inlineValue = (rest, parentIndent) => {
    if (rest === "|" || rest === "|-" || rest === ">") {
      const out = [];
      let blockIndent = -1;
      while (pos < lines.length) {
        const l = lines[pos];
        if (l.text !== "" && l.indent <= parentIndent) break;
        if (l.text !== "" && blockIndent < 0) blockIndent = l.indent;
        out.push(l.text === "" ? "" : l.raw.slice(blockIndent));
        pos++;
      }
      return out.join("\n").replace(/\n+$/, "") + (rest === "|" ? "\n" : "");
    }
    if (rest.startsWith("{") || rest.startsWith("[")) {
      let text = rest;
      while (balance(text) > 0 && pos < lines.length) {
        text += " " + lines[pos].text;
        pos++;
      }
      return parseFlow(text);
    }
    return scalar(rest);
  };

  const block = (indent) => {
    skipBlank();
    if (pos >= lines.length) return null;
    const first = lines[pos];
    if (first.text.startsWith("- ") || first.text === "-") return sequence(first.indent);
    return mapping(indent < 0 ? first.indent : first.indent);
  };

  const keyValue = (text) => {
    let key;
    let rest;
    if (text.startsWith('"') || text.startsWith("'")) {
      const end = text.indexOf(text[0], 1);
      key = text.slice(1, end);
      rest = text.slice(end + 1).replace(/^:\s?/, "");
    } else {
      const m = /^([^:]+?):(?:\s+(.*))?$/.exec(text);
      if (!m) throw new Error(`cannot parse line: ${text}`);
      key = m[1];
      rest = m[2] ?? "";
    }
    return { key, rest: rest.trim() };
  };

  const mapping = (indent) => {
    const out = {};
    for (;;) {
      skipBlank();
      if (pos >= lines.length) return out;
      const l = lines[pos];
      if (l.indent < indent || l.text.startsWith("- ")) return out;
      if (l.indent > indent) throw new Error(`unexpected indent at line ${pos + 1}: ${l.raw}`);
      pos++;
      const { key, rest } = keyValue(l.text);
      if (rest === "") {
        skipBlank();
        const next = lines[pos];
        if (next && (next.indent > indent || (next.indent === indent && next.text.startsWith("- ")))) {
          out[key] = block(next.indent);
        } else {
          out[key] = null;
        }
      } else {
        out[key] = inlineValue(rest, indent);
      }
    }
  };

  const sequence = (indent) => {
    const out = [];
    for (;;) {
      skipBlank();
      if (pos >= lines.length) return out;
      const l = lines[pos];
      if (l.indent !== indent || !l.text.startsWith("-")) return out;
      const rest = l.text.slice(1).trim();
      if (rest === "") {
        pos++;
        out.push(block(indent + 2));
      } else if (rest.startsWith("{") || rest.startsWith("[") || !/^[^"'{[][^:]*:(\s|$)/.test(rest)) {
        pos++;
        out.push(inlineValue(rest, indent));
      } else {
        // "- key: value" starts a mapping whose keys sit at indent + 2.
        const itemIndent = indent + 2;
        lines[pos] = { indent: itemIndent, text: rest, raw: " ".repeat(itemIndent) + rest };
        out.push(mapping(itemIndent));
      }
    }
  };

  return block(-1);
}

// ---------- reference model ----------

const spec = parseYaml(readFileSync(specPath, "utf8"));

function resolveRef(node) {
  if (node && typeof node === "object" && typeof node.$ref === "string") {
    const parts = node.$ref.replace(/^#\//, "").split("/");
    let cur = spec;
    for (const p of parts) cur = cur?.[p];
    if (cur === undefined) throw new Error(`unresolved ${node.$ref}`);
    return cur;
  }
  return node;
}

function refName(node) {
  return node && typeof node.$ref === "string" ? node.$ref.split("/").pop() : null;
}

function typeOf(schema) {
  if (!schema) return "any";
  const name = refName(schema);
  if (name) return name;
  if (Array.isArray(schema.enum)) return schema.enum.join(" | ");
  if (schema.type === "array") return `${typeOf(schema.items)}[]`;
  let t = schema.type ?? "object";
  if (schema.format) t += ` (${schema.format})`;
  if (schema.contentEncoding) t += ` (${schema.contentEncoding})`;
  return t;
}

function constraints(schema) {
  const out = [];
  if (!schema) return out;
  if (schema.pattern) out.push(`pattern ${schema.pattern}`);
  if (schema.maxLength !== undefined) out.push(`max length ${schema.maxLength}`);
  if (schema.minimum !== undefined) out.push(`min ${schema.minimum}`);
  if (schema.maximum !== undefined) out.push(`max ${schema.maximum}`);
  if (schema.minItems !== undefined) out.push(`at least ${schema.minItems} item`);
  if (schema.maxItems !== undefined) out.push(`at most ${schema.maxItems} items`);
  if (schema.default !== undefined) out.push(`default ${schema.default}`);
  if (schema.items && Array.isArray(schema.items.enum)) out.push(`one of ${schema.items.enum.join(", ")}`);
  return out;
}

function fieldsOf(schemaNode) {
  const schema = resolveRef(schemaNode);
  if (!schema || !schema.properties) return [];
  const required = new Set(schema.required ?? []);
  return Object.entries(schema.properties).map(([name, prop]) => ({
    name,
    type: typeOf(prop),
    required: required.has(name),
    description: (prop && prop.description) || "",
    constraints: constraints(prop),
  }));
}

function param(p) {
  const r = resolveRef(p);
  return {
    name: r.name,
    in: r.in,
    required: Boolean(r.required),
    type: typeOf(r.schema),
    description: r.description ?? "",
    constraints: constraints(r.schema),
  };
}

const methods = ["get", "post", "put", "patch", "delete"];
const endpoints = [];
for (const [path, item] of Object.entries(spec.paths)) {
  const shared = (item.parameters ?? []).map(param);
  for (const method of methods) {
    const op = item[method];
    if (!op) continue;
    const params = [...shared, ...(op.parameters ?? []).map(param)];
    const content = op.requestBody?.content?.["application/json"]?.schema;
    const responses = Object.entries(op.responses ?? {}).map(([code, res]) => {
      const named = refName(res);
      const r = resolveRef(res);
      const media = r.content ? Object.keys(r.content)[0] : null;
      const schema = media ? r.content[media]?.schema : undefined;
      return {
        code,
        description: named === "Error" ? "Error envelope" : String(r.description ?? ""),
        schema: named === "Error" ? "Error" : schema ? typeOf(schema) : media && media !== "application/json" ? media : "",
      };
    });
    endpoints.push({
      id: `${method}-${path}`.replace(/[^a-zA-Z0-9]+/g, "-").replace(/-+$/, "").toLowerCase(),
      method: method.toUpperCase(),
      path,
      summary: String(op.summary ?? ""),
      params,
      body: content ? { schema: refName(content) ?? "", fields: fieldsOf(content) } : null,
      responses,
    });
  }
}

const schemaNames = Object.keys(spec.components.schemas);
const schemas = schemaNames
  .filter((n) => spec.components.schemas[n].type === "object")
  .map((name) => ({ name, fields: fieldsOf(spec.components.schemas[name]) }));

const errorSchema = spec.components.responses.Error.content["application/json"].schema;
schemas.push({ name: "Error", fields: fieldsOf(errorSchema) });
const errorCodes = spec.components.responses.Error.content["application/json"].schema.properties.code.enum;
const eventTypes = spec.components.schemas.EventType.enum;
const permissions = spec.components.schemas.Permission?.enum ?? [];

const reference = {
  title: spec.info.title,
  version: String(spec.info.version),
  description: String(spec.info.description).trim(),
  auth: String(spec.components.securitySchemes.bearer.description ?? ""),
  endpoints,
  schemas,
  errorCodes,
  eventTypes,
  permissions,
};

const header = `// Generated by scripts/gen-openapi.mjs from openapi/openapi.yaml. Do not edit.
import type { ApiReference } from "@/lib/openapi-types";

export const apiReference: ApiReference = `;
mkdirSync(dirname(outPath), { recursive: true });
writeFileSync(outPath, header + JSON.stringify(reference, null, 2) + ";\n");
console.log(`gen-openapi: ${endpoints.length} endpoints, ${schemas.length} schemas, ${errorCodes.length} error codes, ${eventTypes.length} events, ${permissions.length} permissions`);
