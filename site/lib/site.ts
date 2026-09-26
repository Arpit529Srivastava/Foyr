// Never hardcode domains (ADR-015): everything reads from env with sane defaults.
export const siteUrl = process.env.NEXT_PUBLIC_SITE_URL ?? "https://foyr.online";

/** Only the real domain is indexed; temporary hosts (e.g. *.vercel.app) and previews are hidden from search. */
export const isIndexable = new URL(siteUrl).hostname === "foyr.online";
export const dashboardUrl = process.env.NEXT_PUBLIC_DASHBOARD_URL ?? "https://app.foyr.online";
// Optional public links: each is hidden until its env var is set (no dead links).
export const social = {
  github: process.env.NEXT_PUBLIC_GITHUB_URL,
  x: process.env.NEXT_PUBLIC_X_URL,
  linkedin: process.env.NEXT_PUBLIC_LINKEDIN_URL,
  email: process.env.NEXT_PUBLIC_CONTACT_EMAIL,
};
// Where "Edit on GitHub" points in the docs, e.g. https://github.com/org/repo/edit/main/site
export const docsEditBaseUrl = process.env.NEXT_PUBLIC_DOCS_EDIT_URL;

export const site = {
  name: "Foyr",
  title: "Foyr: Deploy the app you already built. Login included.",
  description:
    "Push your repo. Foyr gives it a URL, a front door and a guest list. Invite people by email, gate features by role. No auth code. No AWS console.",
};

export const mainNav = [
  { label: "Docs", href: "/docs" },
  { label: "Pricing", href: "/pricing" },
  { label: "Changelog", href: "/changelog" },
] as const;

export const productMenu = [
  { label: "Deploy", href: "/docs/deployments", description: "Push to GitHub, get a live URL behind a health check." },
  { label: "Auth & invites", href: "/docs/auth", description: "Magic-link login in front of every app. Invite by email." },
  { label: "Roles", href: "/docs/roles", description: "Owner, admin, member, viewer. Gate features in one call." },
  { label: "Custom domains", href: "/docs/deployments#custom-domains", description: "Bring your own domain. TLS is automatic." },
] as const;

const askPrompt = encodeURIComponent(
  "What is Foyr (foyr.online)? It deploys apps from GitHub with login, invites and roles built in. How does it work?",
);

export const askLinks = [
  { label: "ChatGPT", href: `https://chatgpt.com/?q=${askPrompt}` },
  { label: "Claude", href: `https://claude.ai/new?q=${askPrompt}` },
  { label: "Perplexity", href: `https://www.perplexity.ai/search?q=${askPrompt}` },
  { label: "Gemini", href: `https://gemini.google.com/app?q=${askPrompt}` },
] as const;
