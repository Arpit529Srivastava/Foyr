"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { SquarePen } from "lucide-react";
import { docsPages } from "@/lib/docs-nav";
import { docsEditBaseUrl } from "@/lib/site";

const LAST_UPDATED = "26 Sep 2026";

/** Previous / Next cards, "Edit on GitHub" (when configured) and last-updated line. */
export function DocsPager() {
  const pathname = usePathname();
  const i = docsPages.findIndex((p) => p.href === pathname);
  if (i === -1) return null;
  const prev = docsPages[i - 1];
  const next = docsPages[i + 1];
  const slug = pathname === "/docs" ? "" : pathname.replace(/^\/docs\//, "") + "/";
  const editUrl = docsEditBaseUrl ? `${docsEditBaseUrl}/app/docs/${slug}page.mdx` : null;

  return (
    <>
      <div className="mt-14 grid gap-3 sm:grid-cols-2">
        {prev ? (
          <Link
            href={prev.href}
            className="flex flex-col gap-1 rounded-xl border border-d-border bg-d-card px-[18px] py-4 transition-colors hover:border-d-border2"
          >
            <span className="text-xs leading-4 text-d-subtle">Previous</span>
            <span className="text-[15px] leading-5 font-[510] text-d-strong">{prev.title}</span>
          </Link>
        ) : (
          <span className="hidden sm:block" />
        )}
        {next && (
          <Link
            href={next.href}
            className="flex flex-col items-end gap-1 rounded-xl border border-d-border bg-d-card px-[18px] py-4 transition-colors hover:border-d-border2"
          >
            <span className="text-xs leading-4 text-d-subtle">Next</span>
            <span className="text-[15px] leading-5 font-[510] text-d-strong">{next.title}</span>
          </Link>
        )}
      </div>
      <div className="mt-6 flex items-center justify-between gap-3 border-t border-d-border pt-5 text-[13px] leading-5 text-d-subtle">
        {editUrl ? (
          <a
            href={editUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="flex min-h-11 items-center gap-1.5 text-d-muted hover:text-d-strong"
          >
            <SquarePen size={14} aria-hidden="true" />
            Edit on GitHub
          </a>
        ) : (
          <span />
        )}
        <span>Last updated {LAST_UPDATED}</span>
      </div>
    </>
  );
}
