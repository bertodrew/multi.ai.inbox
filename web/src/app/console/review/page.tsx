import type { Metadata } from "next";
import { ReviewView } from "@/components/console/ReviewView";

export const metadata: Metadata = { title: "Review queue" };

export default function ReviewPage() {
  return <ReviewView />;
}
