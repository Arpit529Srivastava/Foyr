import type { MetadataRoute } from "next";
import { isIndexable, siteUrl } from "@/lib/site";

export default function robots(): MetadataRoute.Robots {
  return { rules: { userAgent: "*", ...(isIndexable ? { allow: "/" } : { disallow: "/" }) }, sitemap: `${siteUrl}/sitemap.xml` };
}
