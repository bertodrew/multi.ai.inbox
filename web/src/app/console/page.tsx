import type { Metadata } from "next";
import { InboxesView } from "@/components/console/InboxesView";

export const metadata: Metadata = { title: { absolute: "Console · multi.ai.inbox" } };

export default function InboxesPage() {
  return <InboxesView />;
}
