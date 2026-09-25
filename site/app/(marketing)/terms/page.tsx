import type { Metadata } from "next";
import { ProsePage } from "@/components/prose-page";

export const metadata: Metadata = { title: "Terms", alternates: { canonical: "/terms" } };

// Plain-language terms for the waitlist stage. Replace with reviewed terms before Foyr hosts customer apps.
export default function TermsPage() {
  return (
    <ProsePage eyebrow="Legal" title="Terms" updated="26 Sep 2026">
      <p>Foyr is in early access and is not yet a generally available service.</p>
      <h2>This website</h2>
      <p>
        The information on this site, including the docs, describes Foyr as we are building it. Features, names and prices may
        change before launch.
      </p>
      <h2>The waitlist</h2>
      <p>
        Joining the waitlist is free and doesn&apos;t commit you to anything. It also doesn&apos;t guarantee access by a
        particular date.
      </p>
      <h2>Using Foyr</h2>
      <p>
        Full terms of service will be published before anyone can deploy an app on Foyr, and you&apos;ll be asked to accept them
        then.
      </p>
    </ProsePage>
  );
}
