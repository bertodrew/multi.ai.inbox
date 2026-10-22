import { DocArticle, docMetadata } from "@/components/docs/DocArticle";

const slug = "api-reference";

export const metadata = docMetadata(slug);

export default function Page() {
  return <DocArticle slug={slug} />;
}
