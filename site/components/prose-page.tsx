import type { ReactNode } from "react";
import { Container, Eyebrow } from "@/components/ui";

/** Simple long-form text page (privacy, terms). */
export function ProsePage({
  eyebrow,
  title,
  updated,
  children,
}: {
  eyebrow: string;
  title: string;
  updated: string;
  children: ReactNode;
}) {
  return (
    <section className="pt-16 sm:pt-[104px]">
      <Container>
        <div className="max-w-[720px]">
          <Eyebrow>{eyebrow}</Eyebrow>
          <h1 className="mt-4 text-[36px] leading-[44px] font-[510] tracking-[-0.022em] text-white sm:text-5xl sm:leading-[58px]">
            {title}
          </h1>
          <p className="mt-3 text-sm text-subtle">Last updated {updated}</p>
          <div className="mt-10 text-base leading-7 text-muted [&_a]:text-fg [&_a]:underline [&_a]:decoration-amber [&_a]:underline-offset-[3px] [&_h2]:mt-10 [&_h2]:text-xl [&_h2]:font-[510] [&_h2]:text-white [&_li]:mt-1.5 [&_p]:mt-3 [&_strong]:text-fg [&_ul]:mt-3 [&_ul]:list-disc [&_ul]:pl-6">
            {children}
          </div>
        </div>
      </Container>
    </section>
  );
}
