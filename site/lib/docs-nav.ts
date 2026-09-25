export type DocLink = { title: string; href: string; mono?: boolean };
export type DocGroup = { title?: string; links: DocLink[] };

export const docsNav: DocGroup[] = [
  {
    title: "Getting started",
    links: [
      { title: "Introduction", href: "/docs" },
      { title: "Quickstart", href: "/docs/quickstart" },
    ],
  },
  {
    title: "Concepts",
    links: [
      { title: "How auth works", href: "/docs/auth" },
      { title: "Roles & permissions", href: "/docs/roles" },
      { title: "Deployments", href: "/docs/deployments" },
    ],
  },
  {
    title: "Reference",
    links: [
      { title: "SDK — JavaScript", href: "/docs/sdk/javascript" },
      { title: "SDK — Python", href: "/docs/sdk/python" },
      { title: "foyr.json", href: "/docs/foyr-json", mono: true },
    ],
  },
  {
    links: [
      { title: "Architecture", href: "/docs/architecture" },
      { title: "FAQ", href: "/docs/faq" },
    ],
  },
];

export const docsPages = docsNav.flatMap((g) => g.links.map((l) => ({ ...l, group: g.title })));
