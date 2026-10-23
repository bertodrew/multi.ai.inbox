import type { Metadata } from "next";
import { TicketsView } from "@/components/console/TicketsView";

export const metadata: Metadata = { title: "Tickets" };

export default function TicketsPage() {
  return <TicketsView />;
}
