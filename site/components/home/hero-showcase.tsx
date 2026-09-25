"use client";

import Image from "next/image";
import { useState, type ComponentType } from "react";
import { KeyRound, Rocket, ScrollText, type LucideIcon } from "lucide-react";
import { ScaledFrame } from "@/components/scaled-frame";
import { HeroDeployMockup } from "@/components/mockups/hero-deploy";
import { HeroInviteMockup } from "@/components/mockups/hero-invite";
import { HeroLogsMockup } from "@/components/mockups/hero-logs";
import doorway from "@/public/images/doorway-hero.jpg";

const TABS: { id: string; label: string; icon: LucideIcon; Mockup: ComponentType; alt: string }[] = [
  {
    id: "deploy",
    label: "Deploy",
    icon: Rocket,
    Mockup: HeroDeployMockup,
    alt: "Foyr dashboard showing the deployments of acme-crm: one building, one live, one failed, with the live URL acme-crm.foyrapps.online.",
  },
  {
    id: "invite",
    label: "Invite & roles",
    icon: KeyRound,
    Mockup: HeroInviteMockup,
    alt: "Foyr members page listing people with owner, admin, member and viewer roles, and an invite form.",
  },
  {
    id: "logs",
    label: "Logs",
    icon: ScrollText,
    Mockup: HeroLogsMockup,
    alt: "Foyr build log for a deployment: detected Next.js, built, passed the health check and went live with zero downtime.",
  },
];

export function HeroShowcase() {
  const [active, setActive] = useState("deploy");
  const tab = TABS.find((t) => t.id === active)!;

  return (
    <div className="relative mt-10 flex flex-col items-center overflow-hidden rounded-[20px] pb-6 shadow-[0_0_0_1px_#ffffff14] sm:mt-16 sm:pb-10 lg:h-[812px] lg:pb-0">
      <Image
        src={doorway}
        alt=""
        priority
        placeholder="blur"
        sizes="(min-width: 1200px) 1152px, 100vw"
        className="absolute inset-0 size-full object-cover object-[62%_center] sm:object-center"
      />
      <div
        role="tablist"
        aria-label="Product views"
        className="relative mt-4 flex gap-1 rounded-[10px] border border-white/8 bg-black/65 p-1 backdrop-blur-md sm:mt-8"
      >
        {TABS.map((t) => {
          const selected = t.id === active;
          const Icon = t.icon;
          return (
            <button
              key={t.id}
              type="button"
              role="tab"
              id={`hero-tab-${t.id}`}
              aria-selected={selected}
              aria-controls="hero-panel"
              aria-label={t.label}
              onClick={() => setActive(t.id)}
              onKeyDown={(e) => {
                const i = TABS.findIndex((x) => x.id === active);
                if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
                  const next = TABS[(i + (e.key === "ArrowRight" ? 1 : TABS.length - 1)) % TABS.length];
                  setActive(next.id);
                  document.getElementById(`hero-tab-${next.id}`)?.focus();
                }
              }}
              tabIndex={selected ? 0 : -1}
              className={`relative flex h-11 w-[52px] cursor-pointer items-center justify-center gap-2 rounded-md text-sm font-medium transition-colors duration-300 sm:h-9 sm:w-auto sm:px-3.5 ${
                selected ? "bg-raised text-white" : "text-dim hover:text-fg"
              }`}
            >
              <Icon size={16} aria-hidden="true" />
              <span className="hidden sm:inline">{t.label}</span>
              {selected && (
                <span aria-hidden="true" className="absolute bottom-[3px] left-1/2 -ml-2 h-0.5 w-4 rounded-full bg-amber" />
              )}
            </button>
          );
        })}
      </div>
      <div
        id="hero-panel"
        role="tabpanel"
        aria-labelledby={`hero-tab-${active}`}
        className="relative mt-4 w-full px-4 sm:mt-7 sm:px-8 lg:w-[1040px] lg:px-0"
      >
        <div key={tab.id} className="animate-fade">
          <ScaledFrame width={1040} height={640} label={tab.alt} className="mx-auto">
            <tab.Mockup />
          </ScaledFrame>
        </div>
      </div>
    </div>
  );
}
