"use client";

import { useId, useState } from "react";
import { ChevronDown } from "lucide-react";
import { RevealItem, Stagger } from "@/components/reveal";

export function Faq({ items, defaultOpen = 0 }: { items: { q: string; a: string }[]; defaultOpen?: number | null }) {
  const [open, setOpen] = useState<number | null>(defaultOpen);
  const id = useId();
  return (
    <Stagger stagger={0.06} className="border-t border-line">
      {items.map((item, i) => {
        const isOpen = open === i;
        return (
          <RevealItem key={item.q} className="border-b border-line">
            <h3>
              <button
                type="button"
                aria-expanded={isOpen}
                aria-controls={`${id}-${i}`}
                onClick={() => setOpen(isOpen ? null : i)}
                className="flex min-h-16 w-full cursor-pointer items-center justify-between gap-4 py-3 text-left text-base leading-6 font-medium text-fg"
              >
                {item.q}
                <ChevronDown
                  size={18}
                  aria-hidden="true"
                  className={`shrink-0 text-subtle transition-transform duration-200 ${isOpen ? "rotate-180" : ""}`}
                />
              </button>
            </h3>
            <div
              id={`${id}-${i}`}
              inert={!isOpen}
              className={`grid transition-[grid-template-rows] duration-200 ease-[var(--ease-std)] ${isOpen ? "grid-rows-[1fr]" : "grid-rows-[0fr]"}`}
            >
              <div className="min-h-0 overflow-hidden">
                <p className="pr-12 pb-[22px] text-sm leading-[22px] text-muted">{item.a}</p>
              </div>
            </div>
          </RevealItem>
        );
      })}
    </Stagger>
  );
}
