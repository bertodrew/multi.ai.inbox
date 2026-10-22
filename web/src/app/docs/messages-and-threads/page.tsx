import { DocArticle, docMetadata } from "@/components/docs/DocArticle";

const slug = "messages-and-threads";

export const metadata = docMetadata(slug);

export default function Page() {
  return <DocArticle slug={slug} />;
}
