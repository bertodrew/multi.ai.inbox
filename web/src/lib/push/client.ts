"use client";

// Web push, browser side.
//
// The server half is POST /v0/push/subscriptions, GET for the devices this key
// registered and DELETE by id or by endpoint (openapi.yaml, and the contract in
// web/docs/push-design.md). It lives on the same API base as every other console
// call, so the browser keeps talking to one origin. The build is configured by
// NEXT_PUBLIC_MAI_VAPID_PUBLIC_KEY: without it there is nothing to subscribe
// with, every entry point reports "unconfigured" and the card says so instead
// of pretending.
//
// iOS needs the app installed on the home screen (16.4 and later) and the
// permission asked for inside a user gesture, so enablePush must be called from
// a click handler, never from an effect.

import type { ConsoleSession } from "@/lib/console/session";
import { ApiError, call, callNoContent } from "@/lib/console/client";
import { PushSubscription as PushSubscriptionResource, PushSubscriptionList } from "@/lib/console/schemas";

/** VAPID public key, base64url, from the build environment. Safe to ship. */
export const VAPID_PUBLIC_KEY: string = process.env.NEXT_PUBLIC_MAI_VAPID_PUBLIC_KEY ?? "";

/** The body this client sends; the server rejects any other field. */
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

/** One device as the API reports it: a label, never the endpoint or its keys. */
export type PushDevice = {
  id: string;
  label: string;
  lastUsedAt: string | null;
};

/** One half is needed to do anything: the key the browser subscribes with. */
export function pushConfigured(): boolean {
  return VAPID_PUBLIC_KEY.length > 0;
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
    const id = await postSubscription(session, created);
    remember(created.endpoint, id);
    return { kind: "on" };
  } catch (err) {
    // A subscription the API does not know about would wake the device for no
    // reason and never show a mail, so it is dropped again.
    if (created) await created.unsubscribe().catch(() => false);
    forget();
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
    forget();
    return { kind: "off" };
  } catch (err) {
    return { kind: "error", detail: message(err) };
  }
}

/** The devices this key registered, as the API reports them. */
export async function listPushDevices(session: ConsoleSession): Promise<PushDevice[]> {
  const res = await call(session, "GET /v0/push/subscriptions", { params: {} }, PushSubscriptionList);
  return res.push_subscriptions.map((sub) => ({
    id: sub.push_subscription_id,
    label: sub.device,
    lastUsedAt: sub.last_used_at ?? null,
  }));
}

/** Drops one device by id, without touching this browser. */
export async function removePushDevice(session: ConsoleSession, id: string): Promise<void> {
  await callNoContent(session, "DELETE /v0/push/subscriptions/{subscription}", { params: { subscription: id } });
}

async function postSubscription(session: ConsoleSession, subscription: PushSubscription): Promise<string> {
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
  const saved = await call(session, "POST /v0/push/subscriptions", { params: {}, body }, PushSubscriptionResource);
  return saved.push_subscription_id;
}

async function deleteSubscription(session: ConsoleSession, endpoint: string): Promise<void> {
  const known = recall(endpoint);
  if (known) {
    // DELETE /{id}: the id came back when this device subscribed.
    try {
      await removePushDevice(session, known);
      return;
    } catch (err) {
      if (!(err instanceof ApiError) || err.status !== 404) throw err;
    }
  }
  // The design allows the endpoint in the body, for a device this tab did not
  // register or when the id was never stored.
  await callNoContent(session, "DELETE /v0/push/subscriptions", { params: {}, body: { endpoint } });
}

// The id of the subscription this browser made, so the next "turn off" is a
// DELETE by id. localStorage is fine: the id is not a capability, the endpoint
// is, and that one never leaves the browser except to the API.
const REMEMBERED = "mai.push.device";

type Remembered = { endpoint: string; id: string };

function remember(endpoint: string, id: string): void {
  if (!id) return;
  try {
    const value: Remembered = { endpoint, id };
    window.localStorage.setItem(REMEMBERED, JSON.stringify(value));
  } catch {
    // storage blocked: the next turn off falls back to DELETE by endpoint
  }
}

function recall(endpoint: string): string {
  try {
    const raw = window.localStorage.getItem(REMEMBERED);
    if (!raw) return "";
    const parsed = JSON.parse(raw) as Remembered;
    return parsed.endpoint === endpoint ? parsed.id : "";
  } catch {
    return "";
  }
}

function forget(): void {
  try {
    window.localStorage.removeItem(REMEMBERED);
  } catch {
    // nothing stored
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
  if (err instanceof ApiError) {
    if (err.code === "push_disabled") return "This API does not accept push notifications yet.";
    if (err.isPermission) return "This key cannot register a device: it needs notification_write.";
  }
  return err instanceof Error ? err.message : "Something went wrong.";
}
