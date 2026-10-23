"use client";

import { useEffect, useState } from "react";
import { disablePush, enablePush, pushConfigured, readPushState, type PushState } from "@/lib/push/client";
import { useStandalone } from "@/lib/pwa/use-standalone";
import { useConsole } from "@/components/console/ConsoleShell";
import { Badge, Button, Card } from "@/components/console/kit";
import { useToast } from "@/components/console/Toast";

// Notifications for the installed app. Every label states what is actually
// possible: with no key in the build the button stays disabled and says so,
// and nothing is sent until the subscription route exists (docs/push-design.md).
export function PushSettings() {
  const { session } = useConsole();
  const toast = useToast();
  const standalone = useStandalone();
  const [state, setState] = useState<PushState | null>(null);
  const [busy, setBusy] = useState(false);

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
    if (next.kind === "on") toast.success("Notifications on", "New mail can reach this device.");
    if (next.kind === "blocked") toast.error("Notifications blocked", "Allow notifications for this site in the browser settings.");
    if (next.kind === "error") toast.error("Could not turn notifications on", next.detail);
  };

  const disable = async () => {
    setBusy(true);
    const next = await disablePush(session);
    setBusy(false);
    setState(next);
    if (next.kind === "off") toast.info("Notifications off", "This device is no longer subscribed.");
    if (next.kind === "error") toast.error("Could not turn notifications off", next.detail);
  };

  const configured = pushConfigured();
  const on = state?.kind === "on";
  const unsupported = state !== null && (state.kind === "unsupported" || state.kind === "blocked" || state.kind === "error");

  let status: string;
  if (state === null) status = "Checking this browser.";
  else if (state.kind === "unconfigured") status = "Not available in this build: the push key and the subscription route are not set.";
  else if (state.kind === "unsupported") status = state.detail;
  else if (state.kind === "blocked") status = "Notifications are blocked for this site. Allow them in the browser settings, then reload.";
  else if (state.kind === "error") status = state.detail;
  else if (state.kind === "on") status = "On for this device. Turn it off here at any time.";
  else status = "Off. The console can ask for permission when you press the button.";

  const label = on ? "Turn off notifications" : configured ? "Turn on notifications" : "Notifications not configured";

  return (
    <Card>
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-display text-lg font-semibold">Notifications</h2>
        {on ? <Badge tone="good">on</Badge> : null}
      </div>
      <p className="mt-2 max-w-2xl text-sm text-muted">{status}</p>
      {!standalone ? (
        <p className="mt-2 max-w-2xl text-sm text-muted">
          On iPhone, add the console to the home screen from the share menu and open it from there: Safari only offers
          notifications to an installed app.
        </p>
      ) : null}
      <p className="mt-2 max-w-2xl text-xs text-muted">
        Permission is asked for when you press the button. The browser gives this device a subscription, and the console
        sends it to the API you are signed in to. Nothing else leaves this device, and no mail is stored on it.
      </p>
      <div className="mt-4">
        <Button
          variant={on ? "secondary" : "primary"}
          busy={busy}
          disabled={!configured || unsupported || state === null}
          onClick={on ? disable : enable}
        >
          {label}
        </Button>
      </div>
    </Card>
  );
}
