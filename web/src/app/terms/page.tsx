import type { Metadata } from "next";
import Link from "next/link";
import { LegalPage, type LegalSection } from "@/components/LegalPage";
import { CONTACT_EMAIL, CONTROLLER, LEGAL_LAST_UPDATED, SITE_NAME } from "@/lib/site";

export const metadata: Metadata = {
  title: "Terms of service",
  description: "Terms of service and acceptable use for multi.ai.inbox. Draft.",
};

const sections: LegalSection[] = [
  {
    title: "Agreement",
    body: (
      <p>
        These terms govern the use of {SITE_NAME} (the &quot;service&quot;), provided by {CONTROLLER.name},{" "}
        {CONTROLLER.address}. By using the service or an API key, the customer accepts these terms. The service is
        intended for businesses and developers, not for consumers.
      </p>
    ),
  },
  {
    title: "The service",
    body: (
      <p>
        The service provides email inboxes for software agents through a REST API and an MCP server: creating inboxes,
        receiving, storing, screening and sending email, and delivering events to webhooks. The service is in staging:
        features can change and there is no uptime commitment or service level agreement.
      </p>
    ),
  },
  {
    title: "Accounts and API keys",
    body: (
      <p>
        The customer is responsible for everything done with its API keys, including by its agents. Keys must be kept
        secret. If a key leaks, the customer must tell us without delay so we can revoke it.
      </p>
    ),
  },
  {
    title: "Acceptable use",
    body: (
      <>
        <p>The customer must not use the service, or let its agents use it, to:</p>
        <ul>
          <li>send cold outreach, that is unsolicited commercial email to people who did not ask to hear from the sender;</li>
          <li>send spam, bulk marketing or newsletters to purchased, rented or scraped lists;</li>
          <li>send phishing, malware, or mail that impersonates another person or organization;</li>
          <li>harvest addresses, or test lists of addresses for validity;</li>
          <li>send content that is illegal or infringes the rights of others;</li>
          <li>get around plan limits, for example by spreading sending across many accounts;</li>
          <li>attack, probe or overload the service or other customers, except as allowed in our <Link href="/security/#disclosure">disclosure policy</Link>.</li>
        </ul>
        <p>
          We may suspend sending or the account, with or without notice, when we reasonably believe these rules are
          broken or the sending reputation of the platform is at risk. We will explain the reason when we can.
        </p>
      </>
    ),
  },
  {
    title: "Customer content",
    body: (
      <p>
        The customer keeps all rights to the email and data it processes through the service. We process that content
        only to provide the service, as described in the <Link href="/privacy/">privacy policy</Link> and the data
        processing agreement.
      </p>
    ),
  },
  {
    title: "Fees",
    body: (
      <p>
        Plans and prices are listed on the <Link href="/pricing/">pricing page</Link> as launch pricing. Billing is not
        live during staging. Paid terms will apply only after the customer agrees to them. [Payment terms, taxes and
        invoicing to be completed.]
      </p>
    ),
  },
  {
    title: "Liability",
    body: (
      <p>
        [Limitation of liability to be drafted by counsel, in line with the governing law.] The gatekeeper and spam
        screening reduce risk but cannot detect every harmful message; the customer remains responsible for the
        actions its agents take.
      </p>
    ),
  },
  {
    title: "Termination and deletion",
    body: (
      <p>
        The customer can stop using the service at any time and delete its inboxes through the API. When an account
        ends, we delete its data as described in the privacy policy, unless the law requires us to keep it.
      </p>
    ),
  },
  {
    title: "Governing law",
    body: (
      <p>
        Estonian law; courts of Tallinn, Estonia, save the mandatory consumer forum. The contractual entity is
        KaiFloor OÜ, registry code EE-1049281, Tallinn, Estonia.
      </p>
    ),
  },
  {
    title: "Changes and contact",
    body: (
      <p>
        We will announce material changes to these terms in advance. Questions: <a href={`mailto:${CONTACT_EMAIL}`}>{CONTACT_EMAIL}</a>.
      </p>
    ),
  },
];

export default function TermsPage() {
  return (
    <LegalPage
      eyebrow="Legal"
      title="Terms of service"
      updated={LEGAL_LAST_UPDATED}
      intro="Short terms, including the acceptable use rules that protect every customer's sending reputation."
      sections={sections}
    />
  );
}
