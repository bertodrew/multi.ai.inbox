"use client";

import { useId } from "react";
import { MAX_ORG_NAME } from "./types";
import { Field, Note, TextInput } from "./parts";

export function StepOrganization({
  value,
  onChange,
  error,
}: {
  value: string;
  onChange: (value: string) => void;
  error?: string | null;
}) {
  const id = useId();

  return (
    <div>
      <h2 className="font-display text-xl font-semibold tracking-tight">Name your organization</h2>
      <p className="mt-2 text-sm text-muted">
        An organization holds your domains, inboxes, API keys and audit log. A company name works, and so does the
        name of a project.
      </p>
      <div className="mt-5">
        <Field
          id={id}
          label="Organization name"
          hint={`Up to ${MAX_ORG_NAME} characters. Shown in the console, and renameable later.`}
          error={error ?? undefined}
        >
          <TextInput
            id={id}
            value={value}
            onChange={(event) => onChange(event.target.value)}
            autoComplete="organization"
            maxLength={MAX_ORG_NAME}
            placeholder="Acme GmbH"
            aria-invalid={error ? true : undefined}
          />
        </Field>
      </div>
      <div className="mt-5">
        <Note>
          Nothing is created on our servers yet. These answers stay in this browser until sign up and the account
          API are connected, so you can go through the flow and see the result without an account.
        </Note>
      </div>
    </div>
  );
}
