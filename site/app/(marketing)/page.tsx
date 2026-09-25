import Image, { type StaticImageData } from "next/image";
import type { ReactNode } from "react";
import { Check, Minus } from "lucide-react";
import { HeroShowcase } from "@/components/home/hero-showcase";
import { CodeTabs, C, F, K, S, type CodeSample } from "@/components/home/code-tabs";
import { Faq } from "@/components/home/faq";
import { FoyrMark } from "@/components/logo";
import { ScaledFrame } from "@/components/scaled-frame";
import { Reveal, RevealItem, Stagger } from "@/components/reveal";
import { ButtonLink, Container, Eyebrow } from "@/components/ui";
import { WaitlistButton } from "@/components/waitlist";
import { StepRepoMockup } from "@/components/mockups/step-repo";
import { StepBoardMockup } from "@/components/mockups/step-board";
import { StepInviteMockup } from "@/components/mockups/step-invite";
import cardLive from "@/public/images/card-live.jpg";
import cardMembers from "@/public/images/card-members.jpg";
import cardDeploys from "@/public/images/card-deploys.jpg";
import ctaBand from "@/public/images/cta-band.jpg";

export default function Home() {
  return (
    <>
      <Hero />
      <TechMarquee />
      <Features />
      <ForDevelopers />
      <HowItWorks />
      <CodeSection />
      <Comparison />
      <FaqSection />
      <FinalCta />
    </>
  );
}

/* ───────────────────────── Hero ───────────────────────── */

function Hero() {
  return (
    <section aria-labelledby="hero-title" className="pt-14 sm:pt-[104px]">
      <Container>
        <Stagger
          stagger={0.08}
          className="grid items-end gap-4 text-center sm:text-left lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)] lg:gap-x-14"
        >
          <RevealItem>
            <h1
              id="hero-title"
              className="text-[36px] leading-[44px] font-[510] tracking-[-0.022em] text-white sm:text-[48px] sm:leading-[58px] lg:text-[60px] lg:leading-[72px]"
            >
              Deploy the app you <br className="hidden lg:inline" />
              already built. <br className="hidden lg:inline" />
              Login included.
            </h1>
          </RevealItem>
          <div className="flex flex-col gap-7 lg:pb-2">
            <RevealItem>
              <p className="text-lg leading-7 text-muted">
                Push your repo. Foyr gives it a URL, a front door and a guest list — invite people by email, gate features by
                role. No auth code. No AWS console.
              </p>
            </RevealItem>
            <RevealItem className="grid grid-cols-2 gap-3 sm:flex">
              <WaitlistButton size="md" className="h-11 sm:h-10" />
              <ButtonLink href="/docs" variant="secondary" className="h-11 sm:h-10">
                Read the docs
              </ButtonLink>
            </RevealItem>
          </div>
        </Stagger>
        <Reveal delay={0.3}>
          <HeroShowcase />
        </Reveal>
      </Container>
    </section>
  );
}

/* ───────────────────────── Tech marquee ───────────────────────── */

const TECH: { name: string; soon?: boolean }[] = [
  { name: "Next.js" },
  { name: "Node.js", soon: true },
  { name: "Express", soon: true },
  { name: "Python", soon: true },
  { name: "FastAPI", soon: true },
  { name: "Docker" },
  { name: "PostgreSQL" },
  { name: "GitHub" },
  { name: "AWS" },
];

function TechMarquee() {
  const row = (hidden: boolean) => (
    <ul aria-hidden={hidden || undefined} className="m-0 flex list-none items-center gap-12 px-6 sm:gap-16 sm:px-8">
      {TECH.map((t) => (
        <li
          key={t.name}
          className="f-tech relative flex h-5 items-center text-[15px] leading-5 font-semibold tracking-[-0.02em] whitespace-nowrap text-faint transition-colors hover:text-fg sm:h-6 sm:text-lg sm:leading-6"
        >
          {t.name}
          {t.soon && (
            <span className="f-soon absolute -top-[22px] left-1/2 -translate-x-1/2 rounded border border-line bg-panel px-[5px] text-[10px] leading-[14px] font-medium tracking-[0.08em] text-dim uppercase">
              Soon
            </span>
          )}
        </li>
      ))}
    </ul>
  );
  return (
    <Reveal as="section" className="pt-16 sm:pt-20">
      <div className="flex flex-col items-center gap-2 sm:gap-3">
        <h2 id="tech-title" className="px-4 text-center text-xs leading-4 font-medium tracking-[0.08em] text-subtle uppercase">
          Works with what you already use
        </h2>
        <div className="f-marquee w-full max-w-[1152px] overflow-hidden pt-4 pb-1 [mask-image:linear-gradient(90deg,transparent,#000_64px,#000_calc(100%-64px),transparent)] sm:pt-[26px] sm:[mask-image:linear-gradient(90deg,transparent,#000_120px,#000_calc(100%-120px),transparent)]">
          <div className="f-track flex w-max animate-marquee">
            {row(false)}
            {row(true)}
          </div>
        </div>
      </div>
    </Reveal>
  );
}

/* ───────────────────────── Section heading ───────────────────────── */

function SectionHeading({ id, eyebrow, title, lead }: { id: string; eyebrow?: string; title: string; lead: string }) {
  return (
    <Reveal className="grid items-end gap-4 border-t border-line pt-8 md:grid-cols-2 md:gap-x-12">
      <div className="flex flex-col items-start gap-4">
        {eyebrow && <Eyebrow>{eyebrow}</Eyebrow>}
        <h2 id={id} className="text-[28px] leading-[34px] font-[510] tracking-[-0.012em] text-fg sm:text-4xl sm:leading-10">
          {title}
        </h2>
      </div>
      <p className="max-w-[460px] text-lg leading-7 text-muted">{lead}</p>
    </Reveal>
  );
}

/* ───────────────────────── What you get ───────────────────────── */

const FEATURES: { label: string; title: string; body: ReactNode }[] = [
  { label: "Deploy", title: "Push to deploy", body: "Connect a GitHub repo. Every push builds and ships automatically." },
  { label: "Auth", title: "Zero auth code", body: "Magic-link login in front of your app. You never write a login page." },
  { label: "Private", title: "Private by default", body: "Only people you invite can open your app. Flip to public anytime." },
  { label: "Invites", title: "Invite by email", body: "Teammates and clients get a link, not a password. Access per app." },
  { label: "Roles", title: "Four built-in roles", body: "Owner, admin, member, viewer. Gate features with one function call." },
  {
    label: "Safe",
    title: "Zero-downtime deploys",
    body: "New versions go live only after health checks pass. Instant rollback.",
  },
  {
    label: "Domains",
    title: "Custom domains",
    body: (
      <>
        Bring <code className="font-mono text-[13px] text-fg">app.yourco.com</code>. TLS certificates are automatic.
      </>
    ),
  },
  { label: "Region", title: "Hosted in Mumbai", body: "Low latency for India, on AWS. More regions later." },
];

function Features() {
  return (
    <section aria-labelledby="get-title" className="pt-24 sm:pt-32">
      <Container>
        <SectionHeading
          id="get-title"
          eyebrow="What you get"
          title="Every app gets a front door."
          lead="Hosting, login and permissions — one push, no glue."
        />
        <Stagger stagger={0.07} className="mt-10 grid gap-3 sm:grid-cols-2 sm:gap-4 lg:grid-cols-4">
          {FEATURES.map((f) => (
            <RevealItem
              key={f.title}
              className="f-card flex min-h-[176px] flex-col justify-between gap-6 rounded-xl border border-line bg-card p-5 sm:h-[210px]"
            >
              <div className="flex flex-col items-start gap-3">
                <div className="f-shimmer rounded border border-line px-1.5 py-0.5 text-[11px] leading-4 font-medium tracking-[0.08em] text-subtle uppercase">
                  {f.label}
                </div>
                <h3 className="text-[15px] leading-5 font-[510] text-white">{f.title}</h3>
              </div>
              <p className="text-sm leading-[22px] text-muted">{f.body}</p>
            </RevealItem>
          ))}
        </Stagger>
      </Container>
    </section>
  );
}

/* ───────────────────────── For developers ───────────────────────── */

function ImageCard({
  image,
  title,
  body,
  children,
}: {
  image: StaticImageData;
  title: string;
  body: string;
  children: ReactNode;
}) {
  return (
    <RevealItem className="flex flex-col">
      <div className="relative flex h-[360px] items-center justify-center overflow-hidden rounded-2xl shadow-[0_0_0_1px_#ffffff14] sm:h-[420px]">
        <Image
          src={image}
          alt=""
          placeholder="blur"
          sizes="(min-width: 1024px) 380px, 100vw"
          className="absolute inset-0 size-full object-cover"
        />
        <div aria-hidden="true" className="relative flex w-[248px] flex-col rounded-xl bg-white p-4 text-black">
          {children}
        </div>
      </div>
      <h3 className="mt-5 text-[15px] leading-5 font-[510] text-white">{title}</h3>
      <p className="mt-1.5 text-sm leading-[22px] text-muted">{body}</p>
    </RevealItem>
  );
}

function Person({ initials, name, role, invited }: { initials: string; name: string; role: string; invited?: string }) {
  return (
    <div className="flex items-center gap-2.5">
      <div className="flex size-7 shrink-0 items-center justify-center rounded-full bg-[#f0f0f0] text-[11px] font-semibold text-[#202020]">
        {initials}
      </div>
      <div className="grow">
        <div className="text-[13px] leading-[18px] font-medium">{name}</div>
        <div className="text-xs leading-4 text-[#6b6b6b]">{role}</div>
      </div>
      {invited && (
        <div className="rounded bg-[#b4530914] px-1.5 py-0.5 text-[11px] leading-4 font-medium whitespace-nowrap text-[#b45309]">
          {invited}
        </div>
      )}
    </div>
  );
}

function ForDevelopers() {
  const bars = [
    { day: "Mon", h: 40 },
    { day: "Tue", h: 20 },
    { day: "Wed", h: 60, peak: true },
    { day: "Thu", h: 40 },
    { day: "Fri", h: 20 },
  ];
  return (
    <section aria-labelledby="dev-title" className="pt-24 sm:pt-32">
      <Container>
        <SectionHeading
          id="dev-title"
          eyebrow="For developers"
          title="Ship it today. Share it safely."
          lead="Put what you built in front of teammates and clients the day you finish it. Foyr keeps it online, keeps it private and keeps a record of every change."
        />
        <Stagger stagger={0.08} className="mt-10 grid gap-10 md:grid-cols-3 md:gap-4">
          <ImageCard
            image={cardLive}
            title="Your app, live"
            body="A URL the moment the build passes, and a quick read on the traffic it gets."
          >
            <div className="flex flex-col gap-3">
              <div className="flex items-center justify-between gap-2">
                <div className="text-[13px] leading-[18px] font-[510]">acme-crm.foyrapps.online</div>
                <div className="flex items-center gap-1.5 text-xs font-medium text-[#15803d]">
                  <span className="size-1.5 rounded-full bg-[#15803d]" />
                  Live
                </div>
              </div>
              <svg width="216" height="56" viewBox="0 0 216 56" className="w-full">
                <path
                  d="M0 44 L18 40 L36 43 L54 31 L72 35 L90 23 L108 27 L126 19 L144 25 L162 13 L180 17 L198 8 L216 11 L216 56 L0 56 Z"
                  fill="#0000000d"
                />
                <path
                  d="M0 44 L18 40 L36 43 L54 31 L72 35 L90 23 L108 27 L126 19 L144 25 L162 13 L180 17 L198 8 L216 11"
                  fill="none"
                  stroke="#000"
                  strokeWidth="1.5"
                  strokeLinejoin="round"
                />
              </svg>
              <div className="text-xs leading-4 text-[#6b6b6b]">Requests, last 24 hours</div>
            </div>
          </ImageCard>
          <ImageCard
            image={cardMembers}
            title="Who's inside"
            body="Everyone with access, their role, and who hasn't opened their invite yet."
          >
            <div className="flex flex-col gap-3">
              <div className="text-xs leading-4 text-[#6b6b6b]">acme-crm access</div>
              <Person initials="PN" name="Priya Nair" role="Owner" />
              <Person initials="RM" name="Rahul Mehta" role="Admin" />
              <Person initials="TB" name="Tom Becker" role="Viewer" invited="Invited 2h ago" />
            </div>
          </ImageCard>
          <ImageCard
            image={cardDeploys}
            title="Every deploy, tracked"
            body="Each push is logged with its commit, author and result. Roll back in one click."
          >
            <div className="flex flex-col gap-4">
              <div className="text-xs leading-4 text-[#6b6b6b]">Deploys this week</div>
              <div className="relative flex h-24 items-end justify-between px-1.5">
                {bars.map((b) => (
                  <div key={b.day} className="relative flex flex-col items-center gap-1.5">
                    {b.peak && (
                      <div className="absolute bottom-[86px] rounded bg-black px-1.5 py-[3px] text-[11px] leading-[14px] font-medium whitespace-nowrap text-white">
                        3 deploys
                      </div>
                    )}
                    <div className={`w-6 rounded ${b.peak ? "bg-[#202020]" : "bg-[#cecece]"}`} style={{ height: b.h }} />
                    <div className={`text-[11px] leading-[14px] ${b.peak ? "font-medium text-[#202020]" : "text-[#6b6b6b]"}`}>
                      {b.day}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </ImageCard>
        </Stagger>
      </Container>
    </section>
  );
}

/* ───────────────────────── How it works ───────────────────────── */

const STEPS = [
  {
    n: "01",
    title: "Connect your repo",
    body: "Install the Foyr GitHub app on the repos you choose. Read-only, per repo.",
    subTitle: "What Foyr can see",
    sub: "Only the repos you pick, and only to read them. Revoke access from GitHub whenever you like.",
    Mockup: StepRepoMockup,
    alt: "Import a repository screen listing GitHub repos with Connect buttons.",
  },
  {
    n: "02",
    title: "Push to deploy",
    body: "We detect Next.js, build it, and ship it behind a health check.",
    subTitle: "If a build fails",
    sub: "The current version keeps serving. You get the full build log and nothing changes for your users.",
    Mockup: StepBoardMockup,
    alt: "Deployment board with queued, building and live columns.",
  },
  {
    n: "03",
    title: "Invite your people",
    body: "Add emails, pick a role, done. They log in with a magic link.",
    subTitle: "What they see",
    sub: "One email with a sign-in link. No password to make, no account to set up.",
    Mockup: StepInviteMockup,
    alt: "Invite form with email chips and a role picker, next to the invite email a teammate receives.",
  },
];

function HowItWorks() {
  return (
    <section aria-labelledby="how-title" className="overflow-x-clip pt-24 sm:pt-32">
      <Container>
        <SectionHeading
          id="how-title"
          eyebrow="How it works"
          title="From repo to guest list in three steps."
          lead="Nothing to configure first. Connect a repo, push, and invite the people who should see it."
        />
        <div className="mt-14 flex flex-col gap-16 sm:mt-[72px] lg:gap-[88px]">
          {STEPS.map((s) => (
            <Reveal key={s.n} className="relative flex flex-col lg:h-[520px] lg:flex-row lg:items-center">
              <div className="flex flex-col lg:w-[420px]">
                <div className="font-mono text-[13px] leading-5 text-subtle">{s.n} / 03</div>
                <h3 className="mt-3 text-2xl leading-8 font-[510] tracking-[-0.012em] text-fg sm:text-[28px] sm:leading-9">
                  {s.title}
                </h3>
                <p className="mt-3 text-base leading-6 text-muted">{s.body}</p>
                <div className="mt-7 mb-5 h-px bg-line" />
                <div className="text-sm leading-5 font-medium text-fg">{s.subTitle}</div>
                <p className="mt-1.5 text-sm leading-[22px] text-muted">{s.sub}</p>
              </div>
              <div className="mt-6 lg:absolute lg:top-0 lg:left-[520px] lg:mt-0 lg:w-[900px]">
                <ScaledFrame width={900} height={520} label={s.alt}>
                  <s.Mockup />
                </ScaledFrame>
              </div>
            </Reveal>
          ))}
        </div>
      </Container>
    </section>
  );
}

/* ───────────────────────── Code ───────────────────────── */

const Line = ({ indent = 0, children }: { indent?: number; children?: ReactNode }) => (
  <span className="block whitespace-pre" style={{ paddingLeft: `${indent}ch` }}>
    {children ?? " "}
  </span>
);

const SAMPLES: CodeSample[] = [
  {
    id: "next",
    label: "Next.js",
    file: "app/api/reports/route.ts",
    source: `import { getUser, hasPermission } from "@foyr/auth";

export async function GET(req: Request) {
  const user = getUser(req);            // { id, email, role }
  if (!hasPermission(user, "reports:export")) {
    return new Response("Forbidden", { status: 403 });
  }
  return Response.json(await exportReports());
}`,
    lines: (
      <>
        <Line>
          <K>import </K>
          {"{ "}
          <F>getUser</F>, <F>hasPermission</F>
          {" } "}
          <K>from </K>
          <S>&quot;@foyr/auth&quot;</S>;
        </Line>
        <Line />
        <Line>
          <K>export async function </K>GET(req: Request) {"{"}
        </Line>
        <Line indent={2}>
          <K>const </K>user = <F>getUser</F>(req);<C>{"            // { id, email, role }"}</C>
        </Line>
        <Line indent={2}>
          <K>if </K>(!<F>hasPermission</F>(user, <S>&quot;reports:export&quot;</S>)) {"{"}
        </Line>
        <Line indent={4}>
          <K>return new </K>Response(<S>&quot;Forbidden&quot;</S>, {"{ status: 403 }"});
        </Line>
        <Line indent={2}>{"}"}</Line>
        <Line indent={2}>
          <K>return </K>Response.json(<K>await </K>exportReports());
        </Line>
        <Line>{"}"}</Line>
      </>
    ),
  },
  {
    id: "express",
    label: "Express",
    file: "server.js",
    source: `import express from "express";
import { getUser, hasPermission } from "@foyr/auth";

const app = express();

app.get("/reports/export", async (req, res) => {
  const user = getUser(req);        // { id, email, role }
  if (!hasPermission(user, "reports:export")) {
    return res.status(403).send("Forbidden");
  }
  res.json(await exportReports());
});`,
    lines: (
      <>
        <Line>
          <K>import </K>express <K>from </K>
          <S>&quot;express&quot;</S>;
        </Line>
        <Line>
          <K>import </K>
          {"{ "}
          <F>getUser</F>, <F>hasPermission</F>
          {" } "}
          <K>from </K>
          <S>&quot;@foyr/auth&quot;</S>;
        </Line>
        <Line />
        <Line>
          <K>const </K>app = express();
        </Line>
        <Line />
        <Line>
          app.get(<S>&quot;/reports/export&quot;</S>, <K>async </K>(req, res) =&gt; {"{"}
        </Line>
        <Line indent={2}>
          <K>const </K>user = <F>getUser</F>(req);<C>{"        // { id, email, role }"}</C>
        </Line>
        <Line indent={2}>
          <K>if </K>(!<F>hasPermission</F>(user, <S>&quot;reports:export&quot;</S>)) {"{"}
        </Line>
        <Line indent={4}>
          <K>return </K>res.status(403).send(<S>&quot;Forbidden&quot;</S>);
        </Line>
        <Line indent={2}>{"}"}</Line>
        <Line indent={2}>
          res.json(<K>await </K>exportReports());
        </Line>
        <Line>{"});"}</Line>
      </>
    ),
  },
  {
    id: "fastapi",
    label: "FastAPI",
    file: "main.py",
    source: `from fastapi import FastAPI, Request, HTTPException
from foyr_auth import get_user, has_permission

app = FastAPI()

@app.get("/reports/export")
async def export(request: Request):
    user = get_user(request)          # { id, email, role }
    if not has_permission(user, "reports:export"):
        raise HTTPException(status_code=403, detail="Forbidden")
    return await export_reports()`,
    lines: (
      <>
        <Line>
          <K>from </K>fastapi <K>import </K>FastAPI, Request, HTTPException
        </Line>
        <Line>
          <K>from </K>foyr_auth <K>import </K>
          <F>get_user</F>, <F>has_permission</F>
        </Line>
        <Line />
        <Line>app = FastAPI()</Line>
        <Line />
        <Line>
          <K>@app.get</K>(<S>&quot;/reports/export&quot;</S>)
        </Line>
        <Line>
          <K>async def </K>export(request: Request):
        </Line>
        <Line indent={4}>
          user = <F>get_user</F>(request)<C>{"          # { id, email, role }"}</C>
        </Line>
        <Line indent={4}>
          <K>if not </K>
          <F>has_permission</F>(user, <S>&quot;reports:export&quot;</S>):
        </Line>
        <Line indent={8}>
          <K>raise </K>HTTPException(status_code=403, detail=<S>&quot;Forbidden&quot;</S>)
        </Line>
        <Line indent={4}>
          <K>return await </K>export_reports()
        </Line>
      </>
    ),
  },
];

function CodeSection() {
  return (
    <section aria-labelledby="code-title" className="pt-24 sm:pt-32">
      <Container>
        <Stagger stagger={0.08} className="grid items-center gap-10 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-x-16">
          <RevealItem className="flex flex-col items-start gap-4">
            <Eyebrow>The only code you write</Eyebrow>
            <h2
              id="code-title"
              className="text-[28px] leading-[34px] font-[510] tracking-[-0.012em] text-fg sm:text-4xl sm:leading-10"
            >
              Auth without the auth code.
            </h2>
            <p className="text-lg leading-7 text-muted">
              Foyr&apos;s proxy handles login before requests reach your app. Read the user from a header — or use our 50-line
              SDK.
            </p>
          </RevealItem>
          <RevealItem className="min-w-0">
            <CodeTabs samples={SAMPLES} />
          </RevealItem>
        </Stagger>
      </Container>
    </section>
  );
}

/* ───────────────────────── Comparison ───────────────────────── */

type Cell = { text?: string; icon?: "yes" | "partial"; strong?: boolean } | null;
const ROWS: { cap: string; foyr: Cell; paas: Cell; auth: Cell }[] = [
  { cap: "Deploy from GitHub", foyr: { icon: "yes" }, paas: { icon: "yes" }, auth: null },
  {
    cap: "Login in front of your app",
    foyr: { icon: "yes", text: "built in" },
    paas: null,
    auth: { icon: "partial", text: "you integrate it" },
  },
  { cap: "Invite people per app", foyr: { icon: "yes" }, paas: null, auth: { text: "partial" } },
  { cap: "Roles & permissions", foyr: { icon: "yes" }, paas: null, auth: { icon: "partial", text: "you build it" } },
  { cap: "Auth code you write", foyr: { text: "none", strong: true }, paas: null, auth: { text: "lots" } },
];

function CellView({ cell, foyr }: { cell: Cell; foyr?: boolean }) {
  if (!cell)
    return (
      <>
        <Minus size={16} className="text-subtle" aria-hidden="true" />
        <span className="sr-only">No</span>
      </>
    );
  return (
    <div className="flex items-center gap-2">
      {cell.icon === "yes" && (
        <Check size={16} className={foyr ? "text-white" : "text-muted"} aria-label={cell.text ? undefined : "Yes"} />
      )}
      {cell.icon === "partial" && <Check size={16} className="text-subtle" aria-hidden="true" />}
      {cell.text && <span className={cell.strong ? "font-semibold text-white" : ""}>{cell.text}</span>}
    </div>
  );
}

function Comparison() {
  return (
    <section aria-labelledby="compare-title" className="pt-24 sm:pt-32">
      <Container>
        <SectionHeading
          id="compare-title"
          title="Hosting and auth, finally in one place."
          lead="Today you pick a host, then an auth provider, then write the glue between them. Foyr is both."
        />
        <Reveal className="relative -mx-4 mt-10 overflow-x-auto px-4 sm:mx-0 sm:px-0">
          <table className="w-full min-w-[680px] border-separate border-spacing-0 text-left text-sm leading-5">
            <thead>
              <tr>
                <th scope="col" className="h-[60px] w-[34%] px-5 text-[13px] font-medium text-subtle">
                  Capability
                </th>
                <th scope="col" className="h-[60px] w-[22%] rounded-t-xl border border-b-0 border-line bg-card px-5">
                  <div className="flex items-center gap-2 text-[15px] font-[510] text-white">
                    <FoyrMark />
                    Foyr
                  </div>
                </th>
                <th scope="col" className="h-[60px] w-[22%] px-5 font-medium text-dim">
                  Vercel / Railway / Render
                </th>
                <th scope="col" className="h-[60px] w-[22%] px-5 font-medium text-dim">
                  + Auth0 / Clerk / Cognito
                </th>
              </tr>
            </thead>
            <tbody>
              {ROWS.map((r, i) => {
                const last = i === ROWS.length - 1;
                const b = `border-t border-line ${last ? "border-b" : ""}`;
                return (
                  <tr key={r.cap}>
                    <th scope="row" className={`h-14 px-5 font-normal text-fg ${b}`}>
                      {r.cap}
                    </th>
                    <td className={`border-x bg-card px-5 text-fg ${b} ${last ? "rounded-b-xl" : ""}`}>
                      <CellView cell={r.foyr} foyr />
                    </td>
                    <td className={`px-5 text-muted ${b}`}>
                      <CellView cell={r.paas} />
                    </td>
                    <td className={`px-5 text-muted ${b}`}>
                      <CellView cell={r.auth} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </Reveal>
      </Container>
    </section>
  );
}

/* ───────────────────────── FAQ ───────────────────────── */

const FAQ = [
  { q: "What kind of apps can I deploy?", a: "Next.js first. Node and Python are next, and any app with a Dockerfile works." },
  {
    q: "Do I need to change my code to get login?",
    a: "No. The proxy handles it before requests reach your app. The SDK is optional.",
  },
  { q: "Who can see my app?", a: "Only people you invite, unless you make it public." },
  { q: "Where is my app hosted?", a: "On AWS in Mumbai, ap-south-1." },
  { q: "How much will it cost?", a: "Free during early access. Simple pricing at launch." },
  { q: "When can I use it?", a: "Join the waitlist. We're onboarding in small batches." },
];

function FaqSection() {
  return (
    <section aria-labelledby="faq-title" className="pt-24 sm:pt-32">
      <Container className="grid items-start gap-10 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-x-16">
        <Reveal className="flex flex-col items-start gap-4">
          <Eyebrow>[ FAQ ]</Eyebrow>
          <h2
            id="faq-title"
            className="text-[28px] leading-[34px] font-[510] tracking-[-0.012em] text-fg sm:text-4xl sm:leading-10"
          >
            Frequently asked questions
          </h2>
          <p className="max-w-[380px] text-base leading-6 text-muted">
            Short answers to what developers ask first. The docs go deeper.
          </p>
        </Reveal>
        <Faq items={FAQ} />
      </Container>
    </section>
  );
}

/* ───────────────────────── Final CTA ───────────────────────── */

function FinalCta() {
  return (
    <section aria-labelledby="cta-title" className="relative mt-24 h-[440px] overflow-hidden sm:mt-32 sm:h-[520px]">
      <Image
        src={ctaBand}
        alt=""
        placeholder="blur"
        sizes="100vw"
        className="absolute inset-0 size-full object-cover object-[70%_center]"
      />
      <div
        aria-hidden="true"
        className="absolute inset-0 bg-[linear-gradient(90deg,#000_0%,#0000_22%,#0000_80%,#000_100%),linear-gradient(180deg,#000_0%,#0000_30%,#0000_70%,#000_100%)]"
      />
      <Container className="relative h-full">
        <Reveal className="flex h-full flex-col items-start justify-center gap-8">
          <h2
            id="cta-title"
            className="max-w-[620px] text-[40px] leading-[48px] font-[510] tracking-[-0.022em] text-white sm:text-[60px] sm:leading-[72px]"
          >
            Every app deserves a front door.
          </h2>
          <div className="flex gap-3">
            <WaitlistButton />
            <ButtonLink href="/docs" variant="secondary">
              Read the docs
            </ButtonLink>
          </div>
        </Reveal>
      </Container>
    </section>
  );
}
