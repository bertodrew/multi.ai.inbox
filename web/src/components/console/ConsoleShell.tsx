"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { createContext, useCallback, useContext, useMemo, useState, type FormEvent, type ReactNode } from "react";
import { ApiError, call, errorMessage } from "@/lib/console/client";
import { Me, SignInInput } from "@/lib/console/schemas";
import { clearSession, defaultApiBase, saveSession, useSession, type ConsoleSession } from "@/lib/console/session";
import { useResource } from "@/lib/console/use-resource";
import { Button, ErrorState, Field, inputClass, LoadingState } from "./kit";
import { ToastProvider, useToast } from "./Toast";

type ConsoleContextValue = {
  session: ConsoleSession;
  me: Me;
  can: (permission: string) => boolean;
  signOut: () => void;
};

const ConsoleContext = createContext<ConsoleContextValue | null>(null);

export function useConsole(): ConsoleContextValue {
  const ctx = useContext(ConsoleContext);
  if (!ctx) throw new Error("useConsole must be used inside <ConsoleShell>");
  return ctx;
}

const nav = [
  { href: "/console/", label: "Inboxes" },
  { href: "/console/compose/", label: "Compose" },
  { href: "/console/review/", label: "Review queue" },
  { href: "/console/keys/", label: "API keys" },
  { href: "/console/webhooks/", label: "Webhooks" },
  { href: "/console/domains/", label: "Domains" },
  { href: "/console/audit/", label: "Audit log" },
];

export function ConsoleShell({ children }: { children: ReactNode }) {
  return (
    <ToastProvider>
      <div className="mx-auto w-full max-w-6xl px-4 py-6 sm:px-6 sm:py-10">
        <Gate>{children}</Gate>
      </div>
    </ToastProvider>
  );
}

function Gate({ children }: { children: ReactNode }) {
  const session = useSession();
  if (session === undefined) return <LoadingState label="Opening the console" />;
  if (session === null) return <SignIn />;
  return <Authenticated session={session}>{children}</Authenticated>;
}

function Authenticated({ session, children }: { session: ConsoleSession; children: ReactNode }) {
  const toast = useToast();
  const me = useResource(
    `${session.apiBase}|${session.apiKey}`,
    (signal) => call(session, "GET /v0/auth/me", { params: {}, signal }, Me),
    "your organization",
  );
  const signOut = useCallback(() => {
    clearSession();
    toast.info("Signed out", "The API key was removed from this tab.");
  }, [toast]);

  const value = useMemo<ConsoleContextValue | null>(
    () =>
      me.data
        ? { session, me: me.data, can: (p: string) => me.data?.permissions.includes(p) ?? false, signOut }
        : null,
    [session, me.data, signOut],
  );

  if (!value) {
    if (me.error) {
      return (
        <div className="mx-auto max-w-lg">
          <ErrorState error={me.error} onRetry={me.reload} what="your organization" />
          <Button className="mt-3" onClick={signOut}>
            Sign out
          </Button>
        </div>
      );
    }
    return <LoadingState label="Checking the API key" />;
  }

  return (
    <ConsoleContext.Provider value={value}>
      <div className="flex flex-col gap-6 lg:flex-row lg:gap-10">
        <ConsoleNav />
        <div className="min-w-0 flex-1">{children}</div>
      </div>
    </ConsoleContext.Provider>
  );
}

function ConsoleNav() {
  const pathname = usePathname();
  const { me, session, signOut } = useConsole();
  const active = (href: string) =>
    href === "/console/" ? pathname === "/console" || pathname === "/console/" || pathname.startsWith("/console/inbox") : pathname.startsWith(href.replace(/\/$/, ""));
  return (
    <aside className="lg:w-56 lg:shrink-0">
      <div className="rounded-xl border border-border bg-card p-3">
        <p className="truncate text-sm font-medium" title={me.name}>
          {me.name || "Your organization"}
        </p>
        <p className="truncate font-mono text-xs text-muted" title={session.apiBase}>
          {me.plan} plan · {session.apiBase.replace(/^https?:\/\//, "")}
        </p>
        <Button variant="ghost" className="mt-2 w-full justify-start px-2" onClick={signOut}>
          Sign out
        </Button>
      </div>
      <nav aria-label="Console" className="mt-3">
        <ul className="grid grid-cols-2 gap-1 sm:grid-cols-4 lg:grid-cols-1">
          {nav.map((l) => (
            <li key={l.href}>
              <Link
                href={l.href}
                aria-current={active(l.href) ? "page" : undefined}
                className={`flex min-h-11 items-center rounded-lg px-3 text-sm ${
                  active(l.href) ? "bg-primary text-primary-fg" : "hover:bg-card"
                }`}
              >
                {l.label}
              </Link>
            </li>
          ))}
        </ul>
      </nav>
    </aside>
  );
}

function SignIn() {
  const toast = useToast();
  const [apiKey, setApiKey] = useState("");
  const [apiBase, setApiBase] = useState(defaultApiBase);
  const [errors, setErrors] = useState<{ apiKey?: string; apiBase?: string }>({});
  const [busy, setBusy] = useState(false);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const parsed = SignInInput.safeParse({ apiKey, apiBase });
    if (!parsed.success) {
      const next: { apiKey?: string; apiBase?: string } = {};
      for (const issue of parsed.error.issues) {
        const field = issue.path[0];
        if (field === "apiKey" && !next.apiKey) next.apiKey = issue.message;
        if (field === "apiBase" && !next.apiBase) next.apiBase = issue.message;
      }
      setErrors(next);
      return;
    }
    setErrors({});
    setBusy(true);
    try {
      const me = await call(parsed.data, "GET /v0/auth/me", { params: {} }, Me);
      if (!saveSession(parsed.data)) {
        toast.error("Could not keep the key", "This browser blocks session storage for the site; allow it and try again.");
        return;
      }
      toast.success("Signed in", `Organization ${me.name || me.organization_id}`);
    } catch (err) {
      const msg = err instanceof ApiError && err.isAuth ? "The API rejected this key." : errorMessage(err);
      setErrors({ apiKey: msg });
      toast.error("Sign in failed", msg);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mx-auto max-w-lg">
      <h1 className="font-display text-3xl font-semibold tracking-tight">Console</h1>
      <p className="mt-2 text-muted">
        Manage inboxes, mail, keys, webhooks and domains with an API key. The console talks to the API straight from
        your browser.
      </p>
      <form onSubmit={submit} noValidate className="mt-6 flex flex-col gap-4 rounded-xl border border-border bg-card p-4 sm:p-6">
        <Field
          label="API key"
          htmlFor="api-key"
          error={errors.apiKey}
          hint="Starts with mai_live_. Use a scoped key with only the permissions you need here."
        >
          <input
            id="api-key"
            type="password"
            autoComplete="off"
            spellCheck={false}
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            className={`${inputClass} font-mono`}
            placeholder="mai_live_..."
            required
          />
        </Field>
        <details className="group">
          <summary className="cursor-pointer text-sm text-muted">API base: {apiBase}</summary>
          <div className="mt-3">
            <Field label="API base URL" htmlFor="api-base" error={errors.apiBase}>
              <input
                id="api-base"
                type="url"
                spellCheck={false}
                value={apiBase}
                onChange={(e) => setApiBase(e.target.value)}
                className={`${inputClass} font-mono`}
              />
            </Field>
          </div>
        </details>
        <div role="note" className="rounded-lg border border-border border-l-4 border-l-accent bg-bg px-4 py-3 text-sm">
          <p className="font-medium">Where the key is kept</p>
          <p className="mt-1 text-muted">
            Only in this tab&apos;s session storage. It is never sent anywhere except the API base above, and it is gone
            when you close the tab or sign out. Anyone using this browser tab can act with the key, so do not sign in on
            a shared computer.
          </p>
        </div>
        <Button type="submit" variant="primary" busy={busy}>
          {busy ? "Checking the key" : "Sign in"}
        </Button>
      </form>
    </div>
  );
}
