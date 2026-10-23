"use client";

/** Explains up front that the signed in key cannot do something. */
export function PermissionNote({ permission, action }: { permission: string; action: string }) {
  return (
    <div role="note" className="rounded-xl border border-border border-l-4 border-l-accent bg-card px-4 py-3 text-sm">
      <p>
        <span className="font-medium">Not available with this key.</span>{" "}
        <span className="text-muted">
          To {action}, sign in with a key that holds the <code className="font-mono text-xs">{permission}</code>{" "}
          permission.
        </span>
      </p>
    </div>
  );
}
