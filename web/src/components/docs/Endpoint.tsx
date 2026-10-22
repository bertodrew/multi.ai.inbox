import type { ApiEndpoint, ApiField, ApiParam, ApiSchema } from "@/lib/openapi-types";

const methodColor: Record<string, string> = {
  GET: "border-success bg-success/10",
  POST: "border-primary bg-primary/10",
  PATCH: "border-accent bg-accent/20",
  PUT: "border-accent bg-accent/20",
  DELETE: "border-danger bg-danger/10",
};

function FieldRows({ rows }: { rows: Array<ApiField | ApiParam> }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[32rem] text-left text-sm">
        <thead>
          <tr className="border-b border-border text-xs text-muted">
            <th scope="col" className="py-2 pr-4 font-medium">Name</th>
            <th scope="col" className="py-2 pr-4 font-medium">Type</th>
            <th scope="col" className="py-2 font-medium">Notes</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => {
            const notes = [
              "in" in r ? `in ${r.in}` : "",
              r.required ? "required" : "",
              r.description,
              ...r.constraints,
            ].filter(Boolean);
            return (
              <tr key={`${"in" in r ? r.in : "body"}-${r.name}`} className="border-b border-border/60 align-top">
                <td className="py-2 pr-4">
                  <code>{r.name}</code>
                </td>
                <td className="py-2 pr-4 font-mono text-xs break-words">{r.type}</td>
                <td className="py-2 text-muted">{notes.join(", ")}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

export function EndpointCard({ endpoint }: { endpoint: ApiEndpoint }) {
  return (
    <section id={endpoint.id} className="my-6 rounded-xl border border-border bg-card p-4 sm:p-5" aria-labelledby={`${endpoint.id}-h`}>
      <h3 id={`${endpoint.id}-h`} className="!mt-0 flex flex-wrap items-center gap-2 font-mono !text-sm">
        <span className={`rounded border px-2 py-0.5 text-xs font-semibold text-fg ${methodColor[endpoint.method] ?? "border-border"}`}>
          {endpoint.method}
        </span>
        <span className="break-all">{endpoint.path}</span>
      </h3>
      <p className="mt-2 mb-3 text-sm">{endpoint.summary}</p>
      {endpoint.params.length > 0 ? (
        <>
          <p className="mb-1 font-mono text-xs tracking-[0.08em] text-muted uppercase">Parameters</p>
          <FieldRows rows={endpoint.params} />
        </>
      ) : null}
      {endpoint.body ? (
        <>
          <p className="mt-4 mb-1 font-mono text-xs tracking-[0.08em] text-muted uppercase">
            Body {endpoint.body.schema ? <span className="normal-case">({endpoint.body.schema})</span> : null}
          </p>
          <FieldRows rows={endpoint.body.fields} />
        </>
      ) : null}
      <p className="mt-4 mb-1 font-mono text-xs tracking-[0.08em] text-muted uppercase">Responses</p>
      <ul className="!list-none !pl-0 text-sm">
        {endpoint.responses.map((r) => (
          <li key={r.code} className="!mb-1">
            <code>{r.code}</code> {r.description}
            {r.schema ? <span className="text-muted">: {r.schema}</span> : null}
          </li>
        ))}
      </ul>
    </section>
  );
}

export function SchemaCard({ schema }: { schema: ApiSchema }) {
  return (
    <section id={`schema-${schema.name.toLowerCase()}`} className="my-6 rounded-xl border border-border bg-card p-4 sm:p-5">
      <h3 className="!mt-0 font-mono !text-sm">{schema.name} object</h3>
      {schema.fields.length === 0 ? (
        <p className="text-sm text-muted">No documented fields.</p>
      ) : (
        <FieldRows rows={schema.fields} />
      )}
    </section>
  );
}
