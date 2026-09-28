import { labelForUseCase } from "@/lib/waitlist";
import { button, details, escapeHtml, eyebrow, features, heading, layout, paragraph, strong, tokens, type Email } from "./layout";

const link = (href: string, label: string) =>
  `<a href="${escapeHtml(href)}" style="color:${tokens.dim};text-decoration:underline;">${escapeHtml(label)}</a>`;

const product: [string, string][] = [
  ["Push to deploy", "Connect a GitHub repo and get a live URL behind a health check."],
  ["Login included", "Magic-link sign-in in front of every app. No auth code to write."],
  ["Invites and roles", "Invite people by email. Owner, admin, member, viewer."],
];

/** Sent to the person who joined, once a sending domain is verified. */
export function welcomeEmail({
  email,
  useCase,
  siteUrl,
  assetBaseUrl,
}: {
  email: string;
  useCase: string;
  siteUrl: string;
  assetBaseUrl?: string;
}): Email {
  const label = labelForUseCase(useCase);
  const host = new URL(siteUrl).host;
  const body = [
    eyebrow("Waitlist"),
    heading("You&#39;re on the list."),
    paragraph(
      `Thanks for signing up. We&#39;re onboarding developers in small batches, and we&#39;ll email ${strong(email)} as soon as your spot opens.`,
    ),
    label ? paragraph(`You told us you&#39;re building ${strong(label.toLowerCase())}. We&#39;ll keep that in mind.`) : "",
    paragraph(`Here&#39;s what you&#39;ll get:`),
    features(product),
    button(`${siteUrl}/docs`, "Read the docs"),
  ].join("\n");
  const footer = `You&#39;re receiving this because you joined the Foyr waitlist at ${link(siteUrl, host)}. Not you? Ignore this email and you won&#39;t hear from us again.`;

  return {
    subject: "You're on the Foyr waitlist",
    html: layout({
      title: "You're on the Foyr waitlist",
      preheader: "We'll email you when your spot opens.",
      body,
      footer,
      siteUrl,
      assetBaseUrl,
    }),
    text: [
      "You're on the list.",
      "",
      `Thanks for signing up. We're onboarding developers in small batches, and we'll email ${email} as soon as your spot opens.`,
      label ? `\nYou told us you're building ${label.toLowerCase()}.` : "",
      "",
      "What you'll get:",
      ...product.map(([t, d]) => `- ${t}: ${d}`),
      "",
      `Read the docs: ${siteUrl}/docs`,
      "",
      `You're receiving this because you joined the Foyr waitlist at ${host}.`,
    ]
      .filter((line, i, all) => !(line === "" && all[i - 1] === ""))
      .join("\n"),
  };
}

/** Sent to the founder for every new sign-up. */
export function founderNotifyEmail({
  email,
  useCase,
  at,
  audienceId,
  siteUrl,
  assetBaseUrl,
}: {
  email: string;
  useCase: string;
  at: Date;
  audienceId: string;
  siteUrl: string;
  assetBaseUrl?: string;
}): Email {
  const when = `${new Intl.DateTimeFormat("en-IN", { dateStyle: "medium", timeStyle: "short", timeZone: "Asia/Kolkata" }).format(at)} IST`;
  const host = new URL(siteUrl).host;
  const rows: [string, string][] = [
    ["Email", email],
    ["Building", labelForUseCase(useCase) ?? "Not given"],
    ["Joined", when],
    ["Source", host],
  ];
  const audienceUrl = `https://resend.com/audiences/${encodeURIComponent(audienceId)}`;
  const body = [
    eyebrow("New sign-up"),
    heading(`Someone joined the waitlist.`),
    paragraph(`${strong(email)} just signed up. Reply to this email to write to them directly.`),
    details(rows),
    button(audienceUrl, "Open audience in Resend"),
  ].join("\n");
  const footer = `Sent by the waitlist form on ${link(siteUrl, host)} because <code style="font-family:inherit;">WAITLIST_NOTIFY_EMAIL</code> is set.`;

  return {
    subject: `New waitlist sign-up: ${email}`,
    html: layout({
      title: "New waitlist sign-up",
      preheader: `${email} joined the Foyr waitlist.`,
      body,
      footer,
      siteUrl,
      assetBaseUrl,
    }),
    text: [
      `${email} joined the Foyr waitlist.`,
      "",
      ...rows.map(([k, v]) => `${k}: ${v}`),
      "",
      `Audience: ${audienceUrl}`,
      "Reply to this email to write to them directly.",
    ].join("\n"),
  };
}
