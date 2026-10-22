// Docs are written once as typed blocks and rendered twice: to HTML pages and
// to Markdown twins (/docs/<slug>.md) for agents. Inline text supports
// `code`, **bold** and [links](/path).

export type CodeSample = {
  label: string;
  lang: string;
  code: string;
};

export type Block =
  | { type: "p"; text: string }
  | { type: "h2"; text: string }
  | { type: "h3"; text: string }
  | { type: "list"; items: string[]; ordered?: boolean }
  | { type: "code"; sample: CodeSample }
  | { type: "tabs"; samples: CodeSample[] }
  | { type: "table"; head: string[]; rows: string[][] }
  | { type: "note"; tone: "info" | "warn"; text: string }
  /** Endpoints from the OpenAPI file, as "METHOD /path" keys. */
  | { type: "endpoints"; keys: string[] }
  /** Object schemas from the OpenAPI file. */
  | { type: "schemas"; names: string[] };

export type DocPage = {
  slug: string;
  title: string;
  description: string;
  blocks: Block[];
};
