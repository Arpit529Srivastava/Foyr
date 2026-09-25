import type { Metadata } from "next";
import { Check } from "lucide-react";
import { Faq } from "@/components/home/faq";
import { Reveal, RevealItem, Stagger } from "@/components/reveal";
import { Container, Eyebrow } from "@/components/ui";
import { WaitlistPill } from "@/components/waitlist";

export const metadata: Metadata = {
  title: "Pricing",
  description: "Foyr is free while we're in early access. Simple paid plans come after launch.",
  alternates: { canonical: "/pricing" },
};

// Launch pricing isn't decided yet: the cards are greyed out and show no prices or limits.
const TIERS = [
  {
    name: "Hobby",
    blurb: "For side projects and demos you share with a few people.",
    features: ["Deploy from GitHub", "Login in front of your app", "Invite people by email", "A foyrapps.online URL"],
  },
  {
    name: "Pro",
    blurb: "For internal tools and client apps that people rely on.",
    features: [
      "Everything in Hobby",
      "Custom domains with automatic TLS",
      "Owner, admin, member and viewer roles",
      "Instant rollback",
    ],
  },
  {
    name: "Team",
    blurb: "For teams shipping several apps together.",
    features: ["Everything in Pro", "Shared workspace for your team", "Priority support"],
  },
];

const BILLING_FAQ = [
  { q: "Is Foyr free right now?", a: "Yes. Everything is free during early access." },
  {
    q: "What happens at launch?",
    a: "We'll email you before any paid plan starts, with the prices and limits. Nothing is charged automatically: you pick a plan, or stay on the free tier if it fits.",
  },
  { q: "Do I need a card to join the waitlist?", a: "No. Just an email address." },
];

export default function PricingPage() {
  return (
    <>
      <section className="pt-16 sm:pt-[104px]">
        <Container>
          <Stagger stagger={0.08} className="flex max-w-[720px] flex-col items-start gap-4">
            <RevealItem>
              <Eyebrow>Pricing</Eyebrow>
            </RevealItem>
            <RevealItem>
              <h1 className="text-[36px] leading-[44px] font-[510] tracking-[-0.022em] text-white sm:text-[60px] sm:leading-[72px]">
                Free while we&apos;re in early access.
              </h1>
            </RevealItem>
            <RevealItem>
              <p className="text-lg leading-7 text-muted">
                Everything Foyr does is free until launch. Simple paid plans come after that.
              </p>
            </RevealItem>
          </Stagger>

          <Stagger stagger={0.08} className="mt-14 grid gap-4 md:grid-cols-3">
            {TIERS.map((t) => (
              <RevealItem key={t.name} className="flex flex-col rounded-xl border border-line bg-card p-6 opacity-60">
                <div className="flex items-center justify-between">
                  <h2 className="text-[15px] leading-5 font-[510] text-white">{t.name}</h2>
                  <span className="rounded border border-line px-1.5 py-0.5 text-[11px] leading-4 font-medium tracking-[0.08em] text-subtle uppercase">
                    Coming soon
                  </span>
                </div>
                <div className="mt-6 flex items-baseline gap-1">
                  <span className="text-4xl leading-10 font-[510] tracking-[-0.012em] text-fg">TBA</span>
                  <span className="text-sm text-subtle">Pricing at launch</span>
                </div>
                <p className="mt-3 text-sm leading-[22px] text-muted">{t.blurb}</p>
                <ul className="mt-6 flex flex-col gap-2.5 border-t border-line pt-6">
                  {t.features.map((f) => (
                    <li key={f} className="flex items-start gap-2.5 text-sm leading-5 text-fg">
                      <Check size={16} className="mt-0.5 shrink-0 text-subtle" aria-hidden="true" />
                      {f}
                    </li>
                  ))}
                </ul>
                <div className="mt-8 flex h-10 items-center justify-center rounded-lg border border-line text-sm font-medium text-subtle">
                  Coming soon
                </div>
              </RevealItem>
            ))}
          </Stagger>
        </Container>
      </section>

      <section aria-labelledby="free-title" className="pt-24 sm:pt-32">
        <Container>
          <Reveal className="flex flex-col items-start gap-6 border-t border-line pt-8 md:flex-row md:items-end md:justify-between">
            <div className="flex flex-col gap-3">
              <h2
                id="free-title"
                className="text-[28px] leading-[34px] font-[510] tracking-[-0.012em] text-fg sm:text-4xl sm:leading-10"
              >
                Get in while it&apos;s free.
              </h2>
              <p className="text-lg leading-7 text-muted">Join the waitlist. We&apos;re onboarding in small batches.</p>
            </div>
            <WaitlistPill className="w-full md:w-[440px]" />
          </Reveal>
        </Container>
      </section>

      <section aria-labelledby="billing-faq" className="pt-24 sm:pt-32">
        <Container className="grid items-start gap-10 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-x-16">
          <Reveal className="flex flex-col items-start gap-4">
            <Eyebrow>[ FAQ ]</Eyebrow>
            <h2
              id="billing-faq"
              className="text-[28px] leading-[34px] font-[510] tracking-[-0.012em] text-fg sm:text-4xl sm:leading-10"
            >
              Billing questions
            </h2>
          </Reveal>
          <Faq items={BILLING_FAQ} />
        </Container>
      </section>
    </>
  );
}
