import { describe, expect, it } from "vitest";
import { escapeHtml } from "./layout";
import { founderNotifyEmail, welcomeEmail } from "./waitlist";

const siteUrl = "https://foyrai.vercel.app";

describe("escapeHtml", () => {
  it.each([
    ["<script>alert(1)</script>", "&lt;script&gt;alert(1)&lt;/script&gt;"],
    [`"quoted" & 'single'`, "&quot;quoted&quot; &amp; &#39;single&#39;"],
    ["plain@example.com", "plain@example.com"],
  ])("escapes %s", (input, expected) => {
    expect(escapeHtml(input)).toBe(expected);
  });
});

describe("welcomeEmail", () => {
  it("addresses the person and links to the docs", () => {
    const m = welcomeEmail({ email: "sam@acme.com", useCase: "internal-tool", siteUrl });
    expect(m.subject).toBe("You're on the Foyr waitlist");
    expect(m.html).toContain("sam@acme.com");
    expect(m.html).toContain(`${siteUrl}/docs`);
    expect(m.html).toContain("internal tool");
    expect(m.text).toContain("sam@acme.com");
    expect(m.text).not.toMatch(/<[a-z]/i);
  });

  it("leaves out the use case line when none was chosen", () => {
    const m = welcomeEmail({ email: "sam@acme.com", useCase: "", siteUrl });
    expect(m.html).not.toContain("You told us");
    expect(m.text).not.toContain("You told us");
  });

  it("ignores unknown use cases", () => {
    const m = welcomeEmail({ email: "sam@acme.com", useCase: "<b>hack</b>", siteUrl });
    expect(m.html).not.toContain("hack");
  });
});

describe("founderNotifyEmail", () => {
  const at = new Date("2026-09-29T06:30:00Z");

  it("lists the sign-up details in IST with a link to the audience", () => {
    const m = founderNotifyEmail({ email: "sam@acme.com", useCase: "saas-mvp", at, audienceId: "aud-1", siteUrl });
    expect(m.subject).toBe("New waitlist sign-up: sam@acme.com");
    expect(m.html).toContain("SaaS MVP");
    expect(m.html).toContain("12:00");
    expect(m.html).toContain("IST");
    expect(m.html).toContain("https://resend.com/audiences/aud-1");
    expect(m.text).toContain("Building: SaaS MVP");
  });

  it("says when no use case was given", () => {
    const m = founderNotifyEmail({ email: "sam@acme.com", useCase: "", at, audienceId: "aud-1", siteUrl });
    expect(m.text).toContain("Building: Not given");
  });
});

describe("banner", () => {
  it("uses the site's doorway light from the site by default", () => {
    const m = welcomeEmail({ email: "sam@acme.com", useCase: "", siteUrl });
    expect(m.html).toContain(`src="${siteUrl}/email/banner.jpg"`);
  });

  it("can load the welcome banner from another host", () => {
    const m = welcomeEmail({ email: "sam@acme.com", useCase: "", siteUrl, assetBaseUrl: "http://localhost:3100" });
    expect(m.html).toContain(`src="http://localhost:3100/email/banner.jpg"`);
  });

  it("can load the banner from another host", () => {
    const m = founderNotifyEmail({
      email: "sam@acme.com",
      useCase: "",
      at: new Date(),
      audienceId: "x",
      siteUrl,
      assetBaseUrl: "http://localhost:3100",
    });
    expect(m.html).toContain(`src="http://localhost:3100/email/banner.jpg"`);
  });
});

describe("copy rules", () => {
  it("never uses em dashes", () => {
    const at = new Date();
    for (const m of [
      welcomeEmail({ email: "a@b.co", useCase: "other", siteUrl }),
      founderNotifyEmail({ email: "a@b.co", useCase: "other", at, audienceId: "x", siteUrl }),
    ]) {
      expect(m.html + m.text + m.subject).not.toContain("—");
    }
  });
});
