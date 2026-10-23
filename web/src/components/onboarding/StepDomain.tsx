"use client";

import { useId } from "react";
import { normalizeDomain, type DomainChoice } from "./types";
import { ChoiceCard, Field, Note, TextInput } from "./parts";

const RECORDS: { type: string; purpose: string }[] = [
  { type: "MX", purpose: "Routes inbound mail to the platform, priority 10." },
  { type: "TXT SPF", purpose: "Lists the servers allowed to send for this domain." },
  { type: "TXT DKIM", purpose: "Carries the key that signs every message you send." },
  { type: "TXT DMARC", purpose: "Tells receivers what to do with mail that fails the checks." },
];

export function StepDomain({
  choice,
  domain,
  onChoice,
  onDomain,
  error,
}: {
  choice: DomainChoice | null;
  domain: string;
  onChoice: (choice: DomainChoice) => void;
  onDomain: (domain: string) => void;
  error?: string | null;
}) {
  const id = useId();

  return (
    <div>
      <h2 className="font-display text-xl font-semibold tracking-tight">Pick a sending domain</h2>
      <p className="mt-2 text-sm text-muted">
        Inboxes send and receive on a domain. Use one you own, or start on the shared test domain and skip DNS
        entirely. Both paths lead to the same console.
      </p>

      <fieldset className="mt-5">
        <legend className="sr-only">Sending domain</legend>
        <div className="flex flex-col gap-3">
          <ChoiceCard
            name="domain-choice"
            value="own"
            checked={choice === "own"}
            onChange={() => onChoice("own")}
            title="Use a domain you own"
            description="Your own name on every address, for example billing@your-company.eu. You publish four DNS records later, when you are ready to send from it."
          />
          <ChoiceCard
            name="domain-choice"
            value="test"
            checked={choice === "test"}
            onChange={() => onChoice("test")}
            title="Use a free test address"
            description="Start on the platform shared test domain. No DNS records, no verification, nothing to wait for. Move the inboxes to your own domain later."
          />
        </div>
      </fieldset>

      {choice === "own" ? (
        <div className="mt-5 flex flex-col gap-4">
          <Field
            id={id}
            label="Domain"
            hint="Paste it as it appears in your registrar. A full URL works too: only the host is kept."
            error={error ?? undefined}
          >
            <TextInput
              id={id}
              value={domain}
              onChange={(event) => onDomain(event.target.value)}
              onBlur={() => onDomain(normalizeDomain(domain))}
              inputMode="url"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              placeholder="your-company.eu"
              aria-invalid={error ? true : undefined}
            />
          </Field>

          <div className="rounded-lg border border-border bg-bg p-4">
            <p className="font-mono text-xs font-medium tracking-[0.08em] text-muted uppercase">
              Four records to publish
            </p>
            <dl className="mt-3 flex flex-col gap-2 text-sm">
              {RECORDS.map((record) => (
                <div key={record.type} className="grid gap-1 sm:grid-cols-[5.5rem_minmax(0,1fr)] sm:gap-3">
                  <dt className="font-mono text-xs text-muted sm:pt-0.5">{record.type}</dt>
                  <dd className="m-0">{record.purpose}</dd>
                </div>
              ))}
            </dl>
            <p className="mt-3 text-xs text-muted">
              Exact values appear in the console once the domain is added. Verification is not part of this setup:
              nothing here waits for DNS.
            </p>
          </div>
        </div>
      ) : null}

      {choice === "test" ? (
        <div className="mt-5">
          <Note>
            No DNS and no verification, so the next step is not blocked. Inbound mail from the public internet can
            be limited on the shared test domain: to try it today, send from another inbox on the platform.
          </Note>
        </div>
      ) : null}

      {error && choice !== "own" ? (
        <p className="mt-4 text-sm text-danger" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}
