import type { MetadataRoute } from "next";
import { docsPages } from "@/lib/docs-nav";
import { siteUrl } from "@/lib/site";

export default function sitemap(): MetadataRoute.Sitemap {
  const pages = ["/", "/pricing", "/changelog", "/privacy", "/terms", ...docsPages.map((p) => p.href)];
  return pages.map((p) => ({ url: `${siteUrl}${p === "/" ? "" : p}`, lastModified: "2026-09-26" }));
}
