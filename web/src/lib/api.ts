import { apiReference } from "@/generated/openapi";
import type { ApiEndpoint, ApiSchema } from "./openapi-types";

/** Looks up "METHOD /path" keys. Fails the build when the spec and the docs drift. */
export function endpointsFor(keys: string[]): ApiEndpoint[] {
  return keys.map((key) => {
    const found = apiReference.endpoints.find((e) => `${e.method} ${e.path}` === key);
    if (!found) throw new Error(`docs reference unknown endpoint "${key}"; update the docs or openapi.yaml`);
    return found;
  });
}

export function schemasFor(names: string[]): ApiSchema[] {
  return names.map((name) => {
    const found = apiReference.schemas.find((s) => s.name === name);
    if (!found) throw new Error(`docs reference unknown schema "${name}"`);
    return found;
  });
}
