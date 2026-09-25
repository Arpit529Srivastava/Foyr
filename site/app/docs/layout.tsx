import type { Metadata } from "next";
import { DocsShell } from "@/components/docs/docs-shell";
import { buildSearchIndex } from "@/lib/docs-index";

export const metadata: Metadata = {
  title: { default: "Docs", template: "%s · Foyr Docs" },
  description: "How Foyr works: deploy from GitHub, login in front of every app, invites, roles and the SDK.",
};

export default function DocsLayout({ children }: LayoutProps<"/docs">) {
  return <DocsShell searchIndex={buildSearchIndex()}>{children}</DocsShell>;
}
