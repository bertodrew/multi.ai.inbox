import { docPages, getDoc } from "@/content/docs";
import { docToMarkdown } from "@/lib/markdown";

// Markdown twin of each docs page at /docs/<slug>.md, written as a static file.
export const dynamic = "force-static";
export const dynamicParams = false;

export function generateStaticParams(): Array<{ mdfile: string }> {
  return docPages.map((p) => ({ mdfile: `${p.slug}.md` }));
}

export async function GET(_request: Request, context: { params: Promise<{ mdfile: string }> }): Promise<Response> {
  const { mdfile } = await context.params;
  const page = getDoc(mdfile.replace(/\.md$/, ""));
  if (!page) return new Response("Not found\n", { status: 404 });
  return new Response(docToMarkdown(page), {
    headers: { "content-type": "text/markdown; charset=utf-8" },
  });
}
