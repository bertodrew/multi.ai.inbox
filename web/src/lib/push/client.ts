"use client";

// Web push, client side only.
//
// The server half of push does not exist yet: openapi.yaml has no subscription
// route, so this build is configured with no VAPID key and no endpoint, and
// every entry point here reports "unconfigured" instead of pretending. The
// contract this module speaks when both variables are set is written down in
// web/docs/push-design.md.
//
// iOS needs the app installed on the home screen (16.4 and later) and the
// permission asked for inside a user gesture, so enablePush must be called from
// a click handler, never from an effect.

import type { ConsoleSession } from "@/lib/console/session";

/** VAPID public key, base64url, from the build environment. Safe to ship. */
export const VAPID_PUBLIC_KEY: string = process.env.NEXT_PUBLIC_MAI_VAPID_PUBLIC_KEY ?? "";

/**
 * Absolute or relative URL of the subscription route. Unset until the route
 * exists: with no endpoint and no key, push stays off in this build.
 */
export const PUSH_ENDPOINT: string = process.env.NEXT_PUBLIC_MAI_PUSH_ENDPOINT ?? "";

/** The body this client sends. Mirrored by the design note and, later, the spec. */
export type PushSubscriptionBody = {
  endpoint: string;
  keys: { p256dh: string; auth: string };
  expiration_time: number | null;
  user_agent: string;
};

export type PushState =
  | { kind: "unconfigured" }
  | { kind: "unsupported"; detail: string }
  | { kind: "blocked" }
  | { kind: "off" }
  | { kind: "on" }
  | { kind: "error"; detail: string };

/** Both halves are needed to do anything: a key to subscribe with, a route to send it to. */
export function pushConfigured(): boolean {
  return VAPID_PUBLIC_KEY.length > 0 && PUSH_ENDPOINT.length > 0;
}

export function notificationPermission(): NotificationPermission | "unsupported" {
  if (typeof window === "undefined" || !("Notification" in window)) return "unsupported";
  return Notification.permission;
}

/** True when the page runs from the home screen icon rather than a browser tab. */
export function isStandalone(): boolean {
  if (typeof window === "undefined") return false;
  const nav = window.navigator as Navigator & { standalone?: boolean };
  return window.matchMedia("(display-mode: standalone)").matches || nav.standalone === true;
}

/** Whether this browser can show notifications from a service worker at all. */
export function pushSupported(): boolean {
  return (
    typeof window !== "undefined" &&
    "serviceWorker" in navigator &&
    "PushManager" in window &&
    "Notification" in window
  );
}

/** The current state, read without asking for anything. */
export async function readPushState(): Promise<PushState> {
  if (!pushConfigured()) return { kind: "unconfigured" };
  if (!pushSupported()) {
    return {
      kind: "unsupported",
      detail: isStandalone()
        ? "This browser has no web push support."
        : "Add the console to your home screen first, then open it from there.",
    };
  }
  if (Notification.permission === "denied") return { kind: "blocked" };
  try {
    const registration = await navigator.serviceWorker.ready;
    const existing = await registration.pushManager.getSubscription();
    return existing ? { kind: "on" } : { kind: "off" };
  } catch (err) {
    return { kind: "error", detail: message(err) };
  }
}

/**
 * Asks for permission, subscribes and registers the subscription with the API.
 * Call it from a click: the permission prompt is dropped when it is not tied to
 * a gesture, and iOS silently refuses it.
 */
export async function enablePush(session: ConsoleSession): Promise<PushState> {
  if (!pushConfigured()) return { kind: "unconfigured" };
  if (!pushSupported()) return readPushState();

  // First, before any await: this is the gesture bound prompt.
  const permission = Notification.permission === "granted" ? "granted" : await Notification.requestPermission();
  if (permission !== "granted") return { kind: "blocked" };

  let created: PushSubscription | null = null;
  try {
    const registration = await navigator.serviceWorker.ready;
    const existing = await registration.pushManager.getSubscription();
    created =
      existing ??
      (await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(VAPID_PUBLIC_KEY),
      }));
    await postSubscription(session, created);
    return { kind: "on" };
  } catch (err) {
    // A subscription the API does not know about would wake the device for no
    // reason and never show a mail, so it is dropped again.
    if (created) await created.unsubscribe().catch(() => false);
    return { kind: "error", detail: message(err) };
  }
}

/** Drops the subscription here and, best effort, at the API. */
export async function disablePush(session: ConsoleSession): Promise<PushState> {
  if (!pushSupported()) return readPushState();
  try {
    const registration = await navigator.serviceWorker.ready;
    const existing = await registration.pushManager.getSubscription();
    if (existing) {
      await deleteSubscription(session, existing.endpoint).catch(() => undefined);
      await existing.unsubscribe();
    }
    return { kind: "off" };
  } catch (err) {
    return { kind: "error", detail: message(err) };
  }
}

function subscriptionUrl(session: ConsoleSession): string {
  // An absolute PUSH_ENDPOINT wins; a path is resolved against the API base the
  // console is signed in to.
  return new URL(PUSH_ENDPOINT, session.apiBase.replace(/\/?$/, "/")).toString();
}

async function postSubscription(session: ConsoleSession, subscription: PushSubscription): Promise<void> {
  const json = subscription.toJSON();
  if (!json.endpoint || !json.keys?.p256dh || !json.keys.auth) {
    throw new Error("The browser returned an incomplete subscription.");
  }
  const body: PushSubscriptionBody = {
    endpoint: json.endpoint,
    keys: { p256dh: json.keys.p256dh, auth: json.keys.auth },
    expiration_time: subscription.expirationTime,
    user_agent: navigator.userAgent,
  };
  await send(session, "POST", body);
}

async function deleteSubscription(session: ConsoleSession, endpoint: string): Promise<void> {
  await send(session, "DELETE", { endpoint });
}

async function send(session: ConsoleSession, method: "POST" | "DELETE", body: unknown): Promise<void> {
  const res = await fetch(subscriptionUrl(session), {
    method,
    headers: {
      Authorization: `Bearer ${session.apiKey}`,
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify(body),
    credentials: "omit",
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(
      res.status === 404 || res.status === 405
        ? "This API does not accept push subscriptions yet."
        : `The API answered ${res.status} ${res.statusText}.`.trim(),
    );
  }
}

// VAPID keys are base64url. PushManager wants raw bytes.
function urlBase64ToUint8Array(base64url: string): Uint8Array<ArrayBuffer> {
  const padded = (base64url + "=".repeat((4 - (base64url.length % 4)) % 4)).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(padded);
  const bytes = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i += 1) bytes[i] = raw.charCodeAt(i);
  return bytes;
}

function message(err: unknown): string {
  return err instanceof Error ? err.message : "Something went wrong.";
}
