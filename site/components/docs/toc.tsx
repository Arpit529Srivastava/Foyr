"use client";

import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";

type Heading = { id: string; text: string; level: 2 | 3 };

/** "On this page": built from the rendered article's h2/h3, highlighting the section in view. */
export function Toc() {
  const pathname = usePathname();
  const [headings, setHeadings] = useState<Heading[]>([]);
  const [active, setActive] = useState<string | null>(null);

  useEffect(() => {
    const els = [...document.querySelectorAll<HTMLHeadingElement>("#docs-content article :is(h2, h3)[id]")];
    // eslint-disable-next-line react-hooks/set-state-in-effect -- headings come from the DOM rendered for this route
    setHeadings(els.map((el) => ({ id: el.id, text: el.textContent ?? "", level: el.tagName === "H2" ? 2 : 3 })));
    setActive(els[0]?.id ?? null);

    const visible = new Map<string, boolean>();
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) visible.set(e.target.id, e.isIntersecting);
        const first = els.find((el) => visible.get(el.id));
        if (first) setActive(first.id);
      },
      { rootMargin: "-80px 0px -60% 0px" },
    );
    els.forEach((el) => io.observe(el));
    return () => io.disconnect();
  }, [pathname]);

  if (headings.length === 0) return null;
  return (
    <nav aria-label="On this page">
      <div className="text-[13px] leading-5 font-medium text-d-strong">On this page</div>
      <ul className="mt-3 flex flex-col gap-0.5 text-[13px] leading-5">
        {headings.map((h) => {
          const isActive = h.id === active;
          return (
            <li key={h.id}>
              <a
                href={`#${h.id}`}
                aria-current={isActive ? "location" : undefined}
                className={`relative block py-[5px] transition-colors ${h.level === 3 ? "pl-7" : "pl-3.5"} ${
                  isActive ? "font-medium text-d-strong" : "text-d-subtle hover:text-d-fg"
                }`}
              >
                {isActive && <span aria-hidden="true" className="absolute top-3 left-0 size-1.5 rounded-full bg-d-accent" />}
                {h.text}
              </a>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
