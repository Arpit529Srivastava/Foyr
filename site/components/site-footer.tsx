import type React from "react";
import Link from "next/link";
import { ArrowUpRight } from "lucide-react";
import { FoyrLockup } from "@/components/logo";
import { askLinks, social } from "@/lib/site";

type FooterLink = { label: string; href: string | undefined };

const columns: { title: string; links: FooterLink[] }[] = [
  {
    title: "Product",
    links: [
      { label: "Deploy", href: "/docs/deployments" },
      { label: "Auth & invites", href: "/docs/auth" },
      { label: "Roles", href: "/docs/roles" },
      { label: "Custom domains", href: "/docs/deployments#custom-domains" },
      { label: "Pricing", href: "/pricing" },
    ],
  },
  {
    title: "Resources",
    links: [
      { label: "Docs", href: "/docs" },
      { label: "Quickstart", href: "/docs/quickstart" },
      { label: "SDK reference", href: "/docs/sdk/javascript" },
      { label: "Changelog", href: "/changelog" },
    ],
  },
  {
    title: "Company",
    links: [
      { label: "Architecture", href: "/docs/architecture" },
      { label: "FAQ", href: "/docs/faq" },
      { label: "Contact", href: social.email && `mailto:${social.email}` },
    ],
  },
  {
    title: "Connect",
    links: [
      { label: "GitHub", href: social.github },
      { label: "X", href: social.x },
      { label: "LinkedIn", href: social.linkedin },
    ],
  },
];

export function SiteFooter() {
  const cols = columns
    .map((c) => ({ ...c, links: c.links.filter((l): l is { label: string; href: string } => !!l.href) }))
    .filter((c) => c.links.length > 0);
  return (
    <footer className="mx-auto w-full max-w-[1200px] px-4 pt-[88px] pb-10 sm:px-6">
      <div
        style={{ "--cols": cols.length } as React.CSSProperties}
        className="grid grid-cols-2 gap-x-8 gap-y-10 lg:grid-cols-[minmax(0,2fr)_repeat(var(--cols),minmax(0,1fr))]"
      >
        <div className="col-span-2 flex flex-col gap-4 lg:col-span-1">
          <FoyrLockup className="self-start" />
          <p className="max-w-[280px] text-sm leading-[22px] text-muted">
            The front door for the apps you already wrote: hosting, login, invites and roles in one push.
          </p>
        </div>
        {cols.map((col) => (
          <nav key={col.title} aria-label={col.title} className="flex flex-col gap-3">
            <div className="text-[13px] leading-5 font-medium text-fg">{col.title}</div>
            {col.links.map((l) => (
              <Link key={l.label} href={l.href} className="text-sm leading-5 text-muted transition-colors hover:text-fg">
                {l.label}
              </Link>
            ))}
          </nav>
        ))}
      </div>

      <div className="mt-14 flex flex-wrap items-center gap-2">
        <span className="mr-1 text-[13px] text-subtle">Ask about Foyr on</span>
        {askLinks.map((a) => (
          <a
            key={a.label}
            href={a.href}
            target="_blank"
            rel="noopener noreferrer"
            className="flex h-8 items-center gap-1.5 rounded-lg border border-line px-2.5 text-[13px] font-medium text-dim transition-colors hover:border-white/13 hover:text-fg"
          >
            {a.label}
            <ArrowUpRight size={13} aria-hidden="true" />
          </a>
        ))}
      </div>

      <div className="mt-8 flex flex-wrap items-center justify-between gap-3 border-t border-line pt-6 text-[13px] leading-5 text-subtle">
        <div className="flex gap-5">
          <span>© 2026 Foyr</span>
          <Link href="/terms" className="transition-colors hover:text-fg">
            Terms
          </Link>
          <Link href="/privacy" className="transition-colors hover:text-fg">
            Privacy
          </Link>
        </div>
        <span>Made in India</span>
      </div>
    </footer>
  );
}
