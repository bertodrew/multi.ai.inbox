import type { Block } from "@/content/types";
import { endpointsFor, schemasFor } from "@/lib/api";
import { slugify } from "@/lib/slug";
import { plain } from "@/lib/inline";
import { CodeBlock } from "../CodeBlock";
import { CodeTabs } from "../CodeTabs";
import { Note } from "../ui";
import { EndpointCard, SchemaCard } from "./Endpoint";
import { Inline } from "./Inline";

export function DocBlocks({ blocks }: { blocks: Block[] }) {
  return (
    <>
      {blocks.map((b, i) => {
        switch (b.type) {
          case "p":
            return (
              <p key={i}>
                <Inline text={b.text} />
              </p>
            );
          case "h2":
            return (
              <h2 key={i} id={slugify(plain(b.text))}>
                <Inline text={b.text} />
              </h2>
            );
          case "h3":
            return (
              <h3 key={i} id={slugify(plain(b.text))}>
                <Inline text={b.text} />
              </h3>
            );
          case "list": {
            const items = b.items.map((item, j) => (
              <li key={j}>
                <Inline text={item} />
              </li>
            ));
            return b.ordered ? <ol key={i}>{items}</ol> : <ul key={i}>{items}</ul>;
          }
          case "code":
            return <CodeBlock key={i} sample={b.sample} />;
          case "tabs":
            return <CodeTabs key={i} samples={b.samples} label="Code samples" />;
          case "table":
            return (
              <div key={i} className="my-6 overflow-x-auto rounded-xl border border-border">
                <table className="w-full min-w-[30rem] text-left text-sm">
                  <thead className="bg-card">
                    <tr>
                      {b.head.map((h) => (
                        <th key={h} scope="col" className="border-b border-border px-3 py-2 font-medium">
                          {h}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {b.rows.map((row, r) => (
                      <tr key={r} className="border-b border-border/60 align-top last:border-0">
                        {row.map((cell, c) => (
                          <td key={c} className="px-3 py-2">
                            <Inline text={cell} />
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            );
          case "note":
            return (
              <Note key={i} tone={b.tone}>
                <Inline text={b.text} />
              </Note>
            );
          case "endpoints":
            return (
              <div key={i}>
                {endpointsFor(b.keys).map((e) => (
                  <EndpointCard key={e.id} endpoint={e} />
                ))}
              </div>
            );
          case "schemas":
            return (
              <div key={i}>
                {schemasFor(b.names).map((s) => (
                  <SchemaCard key={s.name} schema={s} />
                ))}
              </div>
            );
          default: {
            const exhaustive: never = b;
            return exhaustive;
          }
        }
      })}
    </>
  );
}
