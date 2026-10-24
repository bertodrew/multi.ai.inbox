"use client";

import { useEffect, useState } from "react";
import {
  disablePush,
  enablePush,
  listPushDevices,
  pushConfigured,
  readPushState,
  removePushDevice,
  type PushDevice,
  type PushState,
} from "@/lib/push/client";
import { useStandalone } from "@/lib/pwa/use-standalone";
import { useConsole } from "@/components/console/ConsoleShell";
import { useAction, useResource } from "@/lib/console/use-resource";
import { Badge, Button, Card, When } from "@/components/console/kit";
import { useToast } from "@/components/console/Toast";

// Notifications for the installed app. Every label states what is actually
// possible: with no VAPID key in the build the button stays disabled and says
// so, and a device is only registered once the API has stored the subscription
// it came with (docs/push-design.md).
//
// A notification carries the inbox address and a link into the console, never
// the sender, the subject or a line of the body: it is read on a lock screen.
export function PushSettings() {
  const { session, can } = useConsole();
  const toast = useToast();
  const standalone = useStandalone();
  const action = useAction();
  const [state, setState] = useState<PushState | null>(null);
  const [busy, setBusy] = useState(false);

  const configured = pushConfigured();
  const canRegister = can("notification_write");
  const devices = useResource<PushDevice[]>(
    configured && canRegister ? "push devices" : null,
    () => listPushDevices(session),
    "your notification devices",
  );

  useEffect(() => {
    let alive = true;
    void readPushState().then((next) => {
      if (alive) setState(next);
    });
    return () => {
      alive = false;
    };
  }, []);

  const enable = async () => {
    setBusy(true);
    const next = await enablePush(session);
    setBusy(false);
    setState(next);
    if (next.kind === "on") {
      toast.success("Notifications on", "New mail can reach this device.");
      devices.reload();
    }
    if (next.kind === "blocked") toast.error("Notifications blocked", "Allow notifications for this site in the browser settings.");
    if (next.kind === "error") toast.error("Could not turn notifications on", next.detail);
  };

  const disable = async () => {
    setBusy(true);
    const next = await disablePush(session);
    setBusy(false);
    setState(next);
    if (next.kind === "off") {
      toast.info("Notifications off", "This device is no longer subscribed.");
      devices.reload();
    }
    if (next.kind === "error") toast.error("Could not turn notifications off", next.detail);
  };

  const remove = (device: PushDevice) =>
    action.run(`push-${device.id}`, () => removePushDevice(session, device.id), {
      success: `${device.label} is no longer subscribed.`,
      failure: "Could not remove that device",
    });

  const on = state?.kind === "on";
  const unsupported = state !== null && (state.kind === "unsupported" || state.kind === "blocked" || state.kind === "error");

  let status: string;
  if (state === null) status = "Checking this browser.";
  else if (state.kind === "unconfigured") status = "Not available in this build: no push key is set, so there is nothing to subscribe with.";
  else if (state.kind === "unsupported") status = state.detail;
  else if (state.kind === "blocked") status = "Notifications are blocked for this site. Allow them in the browser settings, then reload.";
  else if (state.kind === "error") status = state.detail;
  else if (state.kind === "on") status = "On for this device. Turn it off here at any time.";
  else status = "Off. The console can ask for permission when you press the button.";

  const label = on ? "Turn off notifications" : configured ? "Turn on notifications" : "Notifications not configured";
  const count = devices.data?.length ?? 0;

  return (
    <Card>
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-display text-lg font-semibold">Notifications</h2>
        {on ? <Badge tone="good">on</Badge> : null}
      </div>
      <p className="mt-2 max-w-2xl text-sm text-muted">{status}</p>
      {!canRegister ? (
        <p className="mt-2 max-w-2xl text-sm text-muted">
          This key cannot register a device: it needs the notification_write permission.
        </p>
      ) : null}
      {!standalone ? (
        <p className="mt-2 max-w-2xl text-sm text-muted">
          On iPhone, add the console to the home screen from the share menu and open it from there: Safari only offers
          notifications to an installed app.
        </p>
      ) : null}
      <p className="mt-2 max-w-2xl text-xs text-muted">
        Permission is asked for when you press the button. The browser gives this device a subscription, and the console
        sends it to the API you are signed in to. A notification names the inbox and links here; it never shows the
        sender, the subject or the message. Nothing else leaves this device, and no mail is stored on it.
      </p>
      <div className="mt-4">
        <Button
          variant={on ? "secondary" : "primary"}
          busy={busy}
          disabled={!configured || !canRegister || unsupported || state === null}
          onClick={on ? disable : enable}
        >
          {label}
        </Button>
      </div>
      {canRegister && count > 0 ? (
        <div className="mt-4 border-t border-border pt-3">
          <p className="text-xs uppercase tracking-wide text-muted">
            {count === 1 ? "1 device registered" : `${count} devices registered`}
          </p>
          <ul className="mt-2 space-y-2">
            {(devices.data ?? []).map((device) => (
              <li key={device.id} className="flex flex-wrap items-center justify-between gap-2 text-sm">
                <span>
                  {device.label}
                  <span className="ml-2 text-xs text-muted">
                    {device.lastUsedAt ? <>last notified <When iso={device.lastUsedAt} /></> : "not notified yet"}
                  </span>
                </span>
                <Button variant="ghost" busy={action.busy === `push-${device.id}`} onClick={() => remove(device)}>
                  Remove
                </Button>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </Card>
  );
}
