import { z } from "zod";

const EnvSchema = z.object({
  URL: z.string().url(),
  E2E_ADMIN_USER: z.string().min(1),
  E2E_ADMIN_PASS: z.string().min(1),
  E2E_DOMAINADMIN_USER: z.string().min(1).optional(),
  E2E_DOMAINADMIN_PASS: z.string().min(1).optional(),
  E2E_MAILBOX_USER: z.string().email().optional(),
  E2E_MAILBOX_PASS: z.string().min(1).optional(),
});

export type Env = z.infer<typeof EnvSchema>;

const parsed = EnvSchema.safeParse(process.env);
if (!parsed.success) {
  const fields = parsed.error.issues.map((i) => i.path.join(".")).join(", ");
  throw new Error(`Invalid E2E environment, check: ${fields}`);
}

export const env: Env = parsed.data;
export const baseURL = env.URL.replace(/\/+$/, "");
