import type { Metadata } from "next";
import { ProsePage } from "@/components/prose-page";
import { social } from "@/lib/site";

export const metadata: Metadata = { title: "Privacy", alternates: { canonical: "/privacy" } };

// Plain-language notice for the waitlist stage. Have it reviewed before Foyr hosts customer apps.
export default function PrivacyPage() {
  return (
    <ProsePage eyebrow="Legal" title="Privacy" updated="26 Sep 2026">
      <p>Foyr is in early access. Right now the only personal data we collect is what you give us when you join the waitlist.</p>
      <h2>What we collect</h2>
      <ul>
        <li>Your email address.</li>
        <li>Optionally, what you plan to deploy (for example &quot;Internal tool&quot;).</li>
        <li>Basic, anonymous usage data about this website, such as which pages are visited.</li>
      </ul>
      <h2>How we use it</h2>
      <p>
        To email you when your early-access spot opens and, occasionally, about Foyr&apos;s launch. We don&apos;t sell it or share
        it for advertising.
      </p>
      <h2>Where it&apos;s stored</h2>
      <p>Waitlist emails are stored with our email provider, Resend. This website is hosted on Vercel.</p>
      <h2>Your choices</h2>
      <p>
        Every email we send has an unsubscribe link. To have your data deleted
        {social.email ? (
          <>
            , email <a href={`mailto:${social.email}`}>{social.email}</a>
          </>
        ) : (
          ", reply to any email from us"
        )}
        .
      </p>
    </ProsePage>
  );
}
