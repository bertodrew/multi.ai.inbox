import { DocArticle, docMetadata } from "@/components/docs/DocArticle";

const slug = "errors";

export const metadata = docMetadata(slug);

export default function Page() {
  return <DocArticle slug={slug} />;
}
