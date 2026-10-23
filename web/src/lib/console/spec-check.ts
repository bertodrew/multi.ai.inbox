// Build time guard: the console's hand written schemas must match the spec.
// Called from the console layout, which is rendered during `next build`, so
// drift between openapi/openapi.yaml and the console fails the build.
import { apiReference } from "@/generated/openapi";
import { endpointsFor } from "@/lib/api";
import { CONSOLE_ENDPOINTS } from "./endpoints";
import { EVENT_TYPES, PERMISSIONS } from "./schemas";

function sameSet(name: string, ours: readonly string[], spec: readonly string[]): void {
  const missing = spec.filter((v) => !ours.includes(v));
  const extra = ours.filter((v) => !spec.includes(v));
  if (missing.length || extra.length) {
    throw new Error(
      `console ${name} drifted from openapi.yaml (missing: ${missing.join(", ") || "none"}; extra: ${extra.join(", ") || "none"})`,
    );
  }
}

export function assertConsoleMatchesSpec(): void {
  sameSet("permissions", PERMISSIONS, apiReference.permissions);
  sameSet("event types", EVENT_TYPES, apiReference.eventTypes);
  endpointsFor([...CONSOLE_ENDPOINTS]);
}
