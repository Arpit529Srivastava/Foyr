import "server-only";
import fs from "node:fs";
import path from "node:path";
import GithubSlugger from "github-slugger";
import { docsPages } from "@/lib/docs-nav";
import type { SearchEntry } from "@/components/docs/search";

const DOCS_DIR = path.join(process.cwd(), "app", "docs");

/** Strip markdown/MDX syntax down to readable text for search snippets. */
function plain(s: string) {
  return s
    .replace(/<[^>]+>/g, " ")
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/[`*_>#|]/g, "")
    .replace(/\s+/g, " ")
    .trim();
}

/** Builds the ⌘K index (one entry per page, one per h2/h3) from the MDX sources at build time. */
export function buildSearchIndex(): SearchEntry[] {
  const entries: SearchEntry[] = [];
  for (const page of docsPages) {
    const rel = page.href === "/docs" ? "page.mdx" : path.join(page.href.replace(/^\/docs\//, ""), "page.mdx");
    let src: string;
    try {
      src = fs.readFileSync(path.join(DOCS_DIR, rel), "utf8");
    } catch {
      continue;
    }
    const group = page.group ?? page.title;
    const slugger = new GithubSlugger();
    const lines = src.replace(/```[\s\S]*?```/g, "").split("\n");
    let current: SearchEntry | null = null;
    let pageEntry: SearchEntry | null = null;

    for (const raw of lines) {
      const line = raw.trim();
      if (!line || line.startsWith("import ") || line.startsWith("export ")) continue;
      const h = /^(#{1,3})\s+(.*)$/.exec(line);
      if (h) {
        const text = plain(h[2]);
        if (h[1].length === 1) {
          pageEntry = { page: page.title, href: page.href, group, text: "" };
          entries.push(pageEntry);
          current = pageEntry;
        } else {
          current = { page: page.title, href: `${page.href}#${slugger.slug(text)}`, group, heading: text, text: "" };
          entries.push(current);
        }
        continue;
      }
      if (current && current.text.length < 160 && !line.startsWith("<") && !line.startsWith("|")) {
        current.text = (current.text + " " + plain(line)).trim().slice(0, 200);
      }
    }
  }
  return entries;
}
