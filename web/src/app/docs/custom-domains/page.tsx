import { DocArticle, docMetadata } from "@/components/docs/DocArticle";

const slug = "custom-domains";

export const metadata = docMetadata(slug);

export default function Page() {
  return <DocArticle slug={slug} />;
}
