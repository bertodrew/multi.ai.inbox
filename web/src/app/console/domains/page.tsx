import type { Metadata } from "next";
import { DomainsView } from "@/components/console/DomainsView";

export const metadata: Metadata = { title: "Domains" };

export default function DomainsPage() {
  return <DomainsView />;
}
