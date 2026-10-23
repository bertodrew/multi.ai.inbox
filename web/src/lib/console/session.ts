"use client";

// The console session: API key and API base, kept in sessionStorage only so
// they disappear when the tab closes. Never localStorage, never a cookie.
import { useSyncExternalStore } from "react";
import { z } from "zod";
import { API_URL } from "@/lib/site";

const STORAGE_KEY = "mai.console.session";

const Stored = z.object({ apiKey: z.string().min(1), apiBase: z.string().min(1) });
export type ConsoleSession = z.infer<typeof Stored>;

const listeners = new Set<() => void>();
let cachedRaw: string | null | undefined;
let cached: ConsoleSession | null = null;

function readRaw(): string | null {
  try {
    return window.sessionStorage.getItem(STORAGE_KEY);
  } catch {
    return null; // storage blocked: the user signs in again on each load
  }
}

function snapshot(): ConsoleSession | null {
  const raw = readRaw();
  if (raw === cachedRaw) return cached;
  cachedRaw = raw;
  cached = null;
  if (raw !== null) {
    try {
      const parsed = Stored.safeParse(JSON.parse(raw));
      cached = parsed.success ? parsed.data : null;
    } catch {
      cached = null;
    }
  }
  return cached;
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function notify(): void {
  for (const l of listeners) l();
}

/** Returns false when the browser refused to store the session. */
export function saveSession(session: ConsoleSession): boolean {
  try {
    window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify(session));
    notify();
    return true;
  } catch {
    return false;
  }
}

export function clearSession(): void {
  try {
    window.sessionStorage.removeItem(STORAGE_KEY);
  } catch {
    // nothing stored, nothing to clear
  }
  notify();
}

/** undefined while rendering on the server or hydrating, then the session or null. */
export function useSession(): ConsoleSession | null | undefined {
  return useSyncExternalStore<ConsoleSession | null | undefined>(subscribe, snapshot, () => undefined);
}

/**
 * The API host for this site: NEXT_PUBLIC_MAI_API_URL when set at build time,
 * otherwise api.<site host> (the layout Caddy serves, see deploy/k8s), and the
 * staging API from site.ts when the console runs on localhost.
 */
export function defaultApiBase(): string {
  const fromEnv = process.env.NEXT_PUBLIC_MAI_API_URL;
  if (fromEnv) return fromEnv.replace(/\/+$/, "");
  if (typeof window === "undefined") return API_URL;
  const { protocol, hostname } = window.location;
  if (hostname === "localhost" || hostname === "127.0.0.1" || /^\d+\.\d+\.\d+\.\d+$/.test(hostname)) return API_URL;
  return `${protocol}//api.${hostname.replace(/^www\./, "")}`;
}
