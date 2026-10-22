import type { Metadata } from "next";
import Link from "next/link";
import { LegalPage, type LegalSection } from "@/components/LegalPage";
import { CONTROLLER, LEGAL_LAST_UPDATED, PRIVACY_EMAIL, SITE_NAME } from "@/lib/site";

export const metadata: Metadata = {
  title: "Privacy policy",
  description: "How multi.ai.inbox processes personal data under the GDPR. Draft.",
};

const processors = [
  { name: "Hetzner Online GmbH", purpose: "Hosting: servers, database, stored mail, backups (Object Storage)", location: "Germany (EU)" },
  { name: "Brevo (Sendinblue SAS)", purpose: "SMTP relay for outbound mail to external recipients", location: "France (EU)" },
  { name: "GitHub, Inc.", purpose: "Source code hosting and build pipeline. No customer data.", location: "United States" },
];

const sections: LegalSection[] = [
  {
    title: "Who we are",
    body: (
      <>
        <p>
          {SITE_NAME} is operated by {CONTROLLER.name}, {CONTROLLER.address}, registered at {CONTROLLER.register}, VAT
          ID {CONTROLLER.vat}, represented by {CONTROLLER.representative} (&quot;we&quot;).
        </p>
        <p>
          Contact for data protection questions: <a href={`mailto:${PRIVACY_EMAIL}`}>{PRIVACY_EMAIL}</a>. [Data
          protection officer: to be confirmed whether one is required.]
        </p>
      </>
    ),
  },
  {
    title: "Our two roles",
    body: (
      <>
        <p>
          <strong>Controller.</strong> We are the controller for personal data about our customers and their users
          (account and contact details, billing data, API usage) and about visitors of this website.
        </p>
        <p>
          <strong>Processor.</strong> For the content of email that customers send and receive through the service
          (messages, attachments, addresses of correspondents), the customer is the controller and we process that data
          only on the customer&apos;s documented instructions, under a data processing agreement according to Article 28
          GDPR. Requests from people whose data appears in customer mail should go to the customer; we help the
          customer answer them.
        </p>
      </>
    ),
  },
  {
    title: "Data we process",
    body: (
      <ul>
        <li>Account data: organization name, contact email addresses, plan.</li>
        <li>Credentials: API keys, stored only as SHA-256 hashes; webhook signing secrets.</li>
        <li>
          Customer email content (as processor): inbound and outbound messages, headers, bodies, attachments, labels,
          the email addresses of senders and recipients, and results of spam, DKIM and gatekeeper checks.
        </li>
        <li>Technical data: IP addresses, request metadata and logs of API and SMTP connections.</li>
        <li>Communication: what you write to us by email.</li>
        <li>Billing data, once billing is live: [to be completed with the payment provider].</li>
      </ul>
    ),
  },
  {
    title: "Purposes and legal bases",
    body: (
      <ul>
        <li>
          Providing the service, including storing, screening and delivering mail: performance of a contract, Article
          6(1)(b) GDPR, and for customer email content the customer&apos;s instructions under Article 28.
        </li>
        <li>
          Security, abuse and spam prevention, enforcing limits and the acceptable use rules: legitimate interests,
          Article 6(1)(f), in keeping the service and its sending reputation safe.
        </li>
        <li>Answering your messages: legitimate interests, Article 6(1)(f), or steps before a contract, 6(1)(b).</li>
        <li>Accounting and tax records: legal obligation, Article 6(1)(c).</li>
      </ul>
    ),
  },
  {
    title: "Processors",
    body: (
      <>
        <p>We use these processors. We will update this list before adding a new one.</p>
        <div className="my-4 overflow-x-auto rounded-xl border border-border">
          <table className="w-full min-w-[30rem] text-left text-sm">
            <thead className="bg-card">
              <tr>
                <th scope="col" className="border-b border-border px-3 py-2 font-medium">Processor</th>
                <th scope="col" className="border-b border-border px-3 py-2 font-medium">Purpose</th>
                <th scope="col" className="border-b border-border px-3 py-2 font-medium">Location</th>
              </tr>
            </thead>
            <tbody>
              {processors.map((p) => (
                <tr key={p.name} className="border-b border-border/60 align-top last:border-0">
                  <td className="px-3 py-2">{p.name}</td>
                  <td className="px-3 py-2">{p.purpose}</td>
                  <td className="px-3 py-2">{p.location}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p>
          Outbound mail is, by its nature, delivered to the mail servers of its recipients, wherever they are. That
          transfer is requested by the customer who sends the message.
        </p>
      </>
    ),
  },
  {
    title: "Transfers outside the EU",
    body: (
      <p>
        Customer data and customer email content are stored and processed in the EU. GitHub, in the United States,
        receives source code only, not customer data. [Counsel to confirm the transfer mechanism for any account data
        that may reach GitHub, for example in issue reports.]
      </p>
    ),
  },
  {
    title: "Retention",
    body: (
      <ul>
        <li>
          Customer email content is kept until the customer deletes it or the account ends. Deleting an inbox deletes
          its messages and attachments from the live database immediately.
        </li>
        <li>Database backups are overwritten on rotation after [N] days, so deleted data leaves backups by then.</li>
        <li>Idempotency records of send requests are deleted after 24 hours.</li>
        <li>Server and connection logs are kept for [N] days.</li>
        <li>Account data is kept for the life of the account, and accounting records as long as the law requires.</li>
      </ul>
    ),
  },
  {
    title: "This website",
    body: (
      <p>
        This website sets no cookies, uses no analytics or tracking and loads no third party resources: fonts are
        served from our own server. The web server records technical access data (IP address, time, requested page,
        user agent) for security, kept for [N] days.
      </p>
    ),
  },
  {
    title: "Your rights",
    body: (
      <>
        <p>
          You have the right to access, rectify and erase your data, to restrict or object to processing, and to data
          portability, under Articles 15 to 21 GDPR. Where processing relies on consent, you can withdraw it at any time.
          Write to <a href={`mailto:${PRIVACY_EMAIL}`}>{PRIVACY_EMAIL}</a>.
        </p>
        <p>
          You can also lodge a complaint with a supervisory authority, in particular in the EU country where you live
          or work. [Competent lead authority to be named.]
        </p>
      </>
    ),
  },
  {
    title: "Security",
    body: (
      <p>
        We protect data with technical and organizational measures described on the <Link href="/security/">security page</Link>, including tenant isolation with row level security, hashed API keys, encrypted transport for the API
        and signed webhooks.
      </p>
    ),
  },
  {
    title: "Changes",
    body: (
      <p>
        We will update this policy when our processing changes and show the date of the last update at the top of the
        page. Customers will be told of material changes in advance.
      </p>
    ),
  },
];

export default function PrivacyPage() {
  return (
    <LegalPage
      eyebrow="Legal"
      title="Privacy policy"
      updated={LEGAL_LAST_UPDATED}
      intro="How we handle personal data, written for customers who trust us with their email."
      sections={sections}
    />
  );
}
