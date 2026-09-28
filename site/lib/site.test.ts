import { describe, expect, it, vi } from "vitest";

describe("siteUrl", () => {
  it("defaults to the Vercel URL until foyr.online is live (ADR-021)", async () => {
    vi.stubEnv("NEXT_PUBLIC_SITE_URL", "");
    vi.resetModules();
    const { siteUrl, isIndexable, askLinks } = await import("./site");
    expect(siteUrl).toBe("https://foyrai.vercel.app");
    expect(isIndexable).toBe(false);
    expect(decodeURIComponent(askLinks[0].href)).toContain("(foyrai.vercel.app)");
    vi.unstubAllEnvs();
  });

  it("uses NEXT_PUBLIC_SITE_URL when set, and indexes only foyr.online", async () => {
    vi.stubEnv("NEXT_PUBLIC_SITE_URL", "https://foyr.online");
    vi.resetModules();
    const { siteUrl, isIndexable } = await import("./site");
    expect(siteUrl).toBe("https://foyr.online");
    expect(isIndexable).toBe(true);
    vi.unstubAllEnvs();
  });
});
