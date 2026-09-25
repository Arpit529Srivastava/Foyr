import Link from "next/link";
import type { MDXComponents } from "mdx/types";
import type { ComponentProps } from "react";
import { Callout, CodeFigure, CodeTabs, ProxyDiagram } from "@/components/docs/mdx";

function A({ href = "", children, ...props }: ComponentProps<"a">) {
  const cls = "text-d-fg underline decoration-d-accent underline-offset-[3px] hover:text-d-strong";
  if (href.startsWith("/") || href.startsWith("#")) {
    return (
      <Link href={href} className={cls}>
        {children}
      </Link>
    );
  }
  return (
    <a href={href} className={cls} target="_blank" rel="noopener noreferrer" {...props}>
      {children}
    </a>
  );
}

const components: MDXComponents = {
  h1: (p) => (
    <h1
      className="mt-4 text-[32px] leading-10 font-[510] tracking-[-0.022em] text-d-strong sm:text-[40px] sm:leading-[48px]"
      {...p}
    />
  ),
  h2: (p) => <h2 className="mt-12 scroll-mt-24 text-2xl leading-8 font-[510] tracking-[-0.012em] text-d-strong" {...p} />,
  h3: (p) => <h3 className="mt-8 scroll-mt-24 text-lg leading-7 font-[510] text-d-strong" {...p} />,
  p: (p) => <p className="mt-3 text-base leading-7 text-d-muted" {...p} />,
  a: A,
  strong: (p) => <strong className="font-semibold text-d-fg" {...p} />,
  ul: (p) => <ul className="mt-3 list-disc space-y-1.5 pl-6 text-base leading-7 text-d-muted marker:text-d-subtle" {...p} />,
  ol: (p) => <ol className="mt-3 list-decimal space-y-1.5 pl-6 text-base leading-7 text-d-muted marker:text-d-subtle" {...p} />,
  li: (p) => <li className="pl-1" {...p} />,
  hr: () => <hr className="my-10 border-d-border" />,
  blockquote: (p) => <blockquote className="mt-4 border-l-2 border-d-border2 pl-4 text-d-muted" {...p} />,
  code: ({ className, ...p }) =>
    // Inline code only; fenced blocks are rendered inside <figure> by rehype-pretty-code.
    "data-language" in p || className ? (
      <code className={className} {...p} />
    ) : (
      <code className="rounded bg-d-inline px-[5px] py-0.5 font-mono text-[0.875em] text-d-fg" {...p} />
    ),
  figure: (p) => ("data-rehype-pretty-code-figure" in p ? <CodeFigure {...p} /> : <figure {...p} />),
  table: (p) => (
    <div className="mt-4 overflow-x-auto rounded-xl border border-d-border">
      <table className="w-full min-w-[520px] border-collapse text-left text-sm leading-[22px]" {...p} />
    </div>
  ),
  th: (p) => <th className="h-10 bg-d-card px-4 text-[13px] font-medium text-d-subtle" {...p} />,
  td: (p) => (
    <td
      className="border-t border-d-border px-4 py-3 align-top text-d-muted [&:first-child]:font-medium [&:first-child]:text-d-fg"
      {...p}
    />
  ),
  Callout,
  CodeTabs,
  ProxyDiagram,
  Lead: (p: ComponentProps<"p">) => <p className="mt-3 text-lg leading-7 text-d-muted" {...p} />,
};

export function useMDXComponents(): MDXComponents {
  return components;
}
