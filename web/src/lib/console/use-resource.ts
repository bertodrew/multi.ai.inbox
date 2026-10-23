"use client";

import { useCallback, useEffect, useEffectEvent, useState } from "react";
import { useToast } from "@/components/console/Toast";
import { errorMessage, isAbort } from "./client";

type Result<T> = { reqKey: string; key: string } & ({ ok: true; data: T } | { ok: false; error: unknown });

export type Resource<T> = {
  /** True while the latest request for this key is in flight. */
  loading: boolean;
  /** Data for this key, possibly from the previous load while reloading. */
  data: T | undefined;
  /** Error of the latest finished load, when it failed. */
  error: unknown;
  reload: () => void;
  /** Updates the loaded data in place after a mutation (create, delete). */
  mutate: (update: (current: T) => T) => void;
};

/**
 * Loads data for `key` and reloads when the key changes. Pass null to skip
 * loading (for example while a required parameter is missing). A failed load
 * shows a toast and is returned as `error` for the page's error state.
 */
export function useResource<T>(
  key: string | null,
  load: (signal: AbortSignal) => Promise<T>,
  what: string,
): Resource<T> {
  const toast = useToast();
  const [nonce, setNonce] = useState(0);
  const [result, setResult] = useState<Result<T> | null>(null);
  const reqKey = `${key ?? ""}#${nonce}`;

  const start = useEffectEvent((signal: AbortSignal) => load(signal));
  const report = useEffectEvent((error: unknown) => toast.error(`Could not load ${what}`, errorMessage(error)));

  useEffect(() => {
    if (key === null) return;
    const ctrl = new AbortController();
    start(ctrl.signal).then(
      (data) => {
        if (!ctrl.signal.aborted) setResult({ reqKey, key, ok: true, data });
      },
      (error: unknown) => {
        if (ctrl.signal.aborted || isAbort(error)) return;
        setResult({ reqKey, key, ok: false, error });
        report(error);
      },
    );
    return () => ctrl.abort();
  }, [key, reqKey]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  const mutate = useCallback(
    (update: (current: T) => T) =>
      setResult((r) => (r && r.ok ? { ...r, data: update(r.data) } : r)),
    [],
  );

  const forKey = result !== null && result.key === key ? result : null;
  return {
    loading: key !== null && result?.reqKey !== reqKey,
    data: forKey?.ok ? forKey.data : undefined,
    error: forKey && !forKey.ok && forKey.reqKey === reqKey ? forKey.error : undefined,
    reload,
    mutate,
  };
}

export type Outcome<T> = { ok: true; value: T } | { ok: false; error: unknown };

/**
 * Runs a user action (create, delete, send) with a busy flag, a success toast
 * and an error toast. Never throws: the outcome says whether it worked.
 */
export function useAction() {
  const toast = useToast();
  const [busy, setBusy] = useState<string | null>(null);

  const run = useCallback(
    async <T>(
      id: string,
      action: () => Promise<T>,
      messages: { success?: string; failure: string },
    ): Promise<Outcome<T>> => {
      setBusy(id);
      try {
        const value = await action();
        if (messages.success) toast.success(messages.success);
        return { ok: true, value };
      } catch (error) {
        toast.error(messages.failure, errorMessage(error));
        return { ok: false, error };
      } finally {
        setBusy(null);
      }
    },
    [toast],
  );

  return { busy, run };
}
