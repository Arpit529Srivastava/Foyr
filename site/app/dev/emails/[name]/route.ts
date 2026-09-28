import { founderNotifyEmail, welcomeEmail } from "@/lib/emails/waitlist";
import { siteUrl } from "@/lib/site";

// Local-only previews: /dev/emails/welcome and /dev/emails/notify. 404 everywhere else.
export async function GET(req: Request, ctx: RouteContext<"/dev/emails/[name]">) {
  if (process.env.NODE_ENV !== "development") return new Response("Not found", { status: 404 });
  const { name } = await ctx.params;
  // Banner loads from this dev server, since the site URL may not serve it yet.
  const sample = { email: "sam@acme.com", useCase: "internal-tool", siteUrl, assetBaseUrl: new URL(req.url).origin };
  const email =
    name === "welcome"
      ? welcomeEmail(sample)
      : name === "notify"
        ? founderNotifyEmail({ ...sample, at: new Date(), audienceId: "0d8f002a-4d33-445e-8b9e-ae2ffcb4a848" })
        : null;
  if (!email) return new Response("Not found", { status: 404 });
  return new Response(email.html, { headers: { "Content-Type": "text/html; charset=utf-8" } });
}
