import { describe, expect, it, vi } from "vitest";

describe("siteUrl", () => {
  it("defaults to foyr.online (ADR-022)", async () => {
    vi.stubEnv("NEXT_PUBLIC_SITE_URL", "");
    vi.resetModules();
    const { siteUrl, isIndexable, askLinks } = await import("./site");
    expect(siteUrl).toBe("https://foyr.online");
    expect(isIndexable).toBe(true);
    expect(decodeURIComponent(askLinks[0].href)).toContain("(foyr.online)");
    vi.unstubAllEnvs();
  });

  it("uses NEXT_PUBLIC_SITE_URL when set, and indexes only foyr.online", async () => {
    vi.stubEnv("NEXT_PUBLIC_SITE_URL", "https://foyrai.vercel.app");
    vi.resetModules();
    const { siteUrl, isIndexable } = await import("./site");
    expect(siteUrl).toBe("https://foyrai.vercel.app");
    expect(isIndexable).toBe(false);
    vi.unstubAllEnvs();
  });
});
