import type { Metadata } from "next";
import { ButtonLink, Container, Eyebrow } from "@/components/ui";

export const metadata: Metadata = {
  title: "Page not found",
};

export default function NotFound() {
  return (
    <Container className="py-24">
      <Eyebrow>Error 404</Eyebrow>
      <h1 className="mt-3 font-display text-4xl font-semibold tracking-tight">No box with that number</h1>
      <p className="mt-4 max-w-xl text-lg text-muted">
        The page you asked for does not exist or has moved. Check the address, or start from one of these.
      </p>
      <div className="mt-8 flex flex-wrap gap-3">
        <ButtonLink href="/">Home</ButtonLink>
        <ButtonLink href="/docs/" variant="secondary">
          Docs
        </ButtonLink>
      </div>
    </Container>
  );
}
