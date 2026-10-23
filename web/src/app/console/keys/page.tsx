import type { Metadata } from "next";
import { KeysView } from "@/components/console/KeysView";

export const metadata: Metadata = { title: "API keys" };

export default function KeysPage() {
  return <KeysView />;
}
