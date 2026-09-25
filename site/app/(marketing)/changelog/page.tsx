import type { Metadata } from "next";
import { Reveal, RevealItem, Stagger } from "@/components/reveal";
import { ButtonLink, Container, Eyebrow } from "@/components/ui";

export const metadata: Metadata = {
  title: "Changelog",
  description: "Product, docs and infrastructure updates to Foyr, newest first.",
  alternates: { canonical: "/changelog" },
};

type Entry = {
  date: string;
  iso: string;
  tag: "Product" | "Docs" | "Infra";
  title: string;
  body: string;
  link?: { label: string; href: string };
};

// Newest first. Add an entry for every release.
const ENTRIES: Entry[] = [
  {
    date: "26 Sep 2026",
    iso: "2026-09-26",
    tag: "Product",
    title: "Foyr is being built",
    body: "We're building Foyr: deploy the app you already wrote, with login, invites and roles built in, hosted on AWS in Mumbai. The docs describe how it works, and the waitlist is open while we onboard developers in small batches.",
    link: { label: "Read the docs", href: "/docs" },
  },
];

export default function ChangelogPage() {
  return (
    <section className="pt-16 sm:pt-[104px]">
      <Container>
        <Stagger stagger={0.08} className="flex max-w-[720px] flex-col items-start gap-4">
          <RevealItem>
            <Eyebrow>Changelog</Eyebrow>
          </RevealItem>
          <RevealItem>
            <h1 className="text-[36px] leading-[44px] font-[510] tracking-[-0.022em] text-white sm:text-[60px] sm:leading-[72px]">
              What&apos;s new in Foyr
            </h1>
          </RevealItem>
          <RevealItem>
            <p className="text-lg leading-7 text-muted">Product, docs and infrastructure updates, newest first.</p>
          </RevealItem>
        </Stagger>

        <ol className="mt-16 border-t border-line">
          {ENTRIES.map((e) => (
            <Reveal
              as="li"
              key={e.iso + e.title}
              className="grid gap-4 border-b border-line py-10 md:grid-cols-[200px_minmax(0,1fr)] md:gap-10"
            >
              <div className="md:sticky md:top-[89px] md:self-start">
                <time dateTime={e.iso} className="font-mono text-[13px] leading-5 text-subtle">
                  {e.date}
                </time>
              </div>
              <div className="flex max-w-[640px] flex-col items-start gap-3">
                <span className="rounded border border-line px-1.5 py-0.5 text-[11px] leading-4 font-medium tracking-[0.08em] text-subtle uppercase">
                  {e.tag}
                </span>
                <h2 className="text-2xl leading-8 font-[510] tracking-[-0.012em] text-white">{e.title}</h2>
                <p className="text-base leading-7 text-muted">{e.body}</p>
                {e.link && (
                  <ButtonLink href={e.link.href} variant="secondary" size="sm" className="mt-2">
                    {e.link.label}
                  </ButtonLink>
                )}
              </div>
            </Reveal>
          ))}
        </ol>
        <p className="mt-8 text-sm text-subtle">That&apos;s everything so far.</p>
      </Container>
    </section>
  );
}
