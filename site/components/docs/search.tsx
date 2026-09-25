"use client";

import { useRouter } from "next/navigation";
import { useEffect, useId, useMemo, useRef, useState, type ReactNode } from "react";
import { FileText, Hash, Search } from "lucide-react";

export type SearchEntry = {
  page: string;
  href: string;
  group: string;
  heading?: string;
  text: string;
};

function highlight(text: string, q: string, strongClass: string): ReactNode {
  if (!q) return text;
  const i = text.toLowerCase().indexOf(q.toLowerCase());
  if (i === -1) return text;
  return (
    <>
      {text.slice(0, i)}
      <strong className={`font-semibold ${strongClass}`}>{text.slice(i, i + q.length)}</strong>
      {text.slice(i + q.length)}
    </>
  );
}

function snippet(text: string, q: string) {
  const i = text.toLowerCase().indexOf(q.toLowerCase());
  if (i <= 40) return text;
  return "…" + text.slice(i - 30);
}

export function DocsSearch({
  open,
  onClose,
  index,
  theme,
}: {
  open: boolean;
  onClose: () => void;
  index: SearchEntry[];
  theme: "dark" | "light";
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const router = useRouter();
  const listId = useId();
  const [q, setQ] = useState("");
  const [sel, setSel] = useState(0);

  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) {
      d.showModal();
      inputRef.current?.focus();
    }
    if (!open && d.open) d.close();
  }, [open]);

  const results = useMemo(() => {
    const query = q.trim().toLowerCase();
    const list = query
      ? index.filter((e) => `${e.page} ${e.heading ?? ""} ${e.text}`.toLowerCase().includes(query))
      : index.filter((e) => !e.heading);
    // Titles and headings that match rank above body-text matches.
    const score = (e: SearchEntry) =>
      (e.heading ?? e.page).toLowerCase().includes(query) ? 0 : e.page.toLowerCase().includes(query) ? 1 : 2;
    return query ? [...list].sort((a, b) => score(a) - score(b)).slice(0, 12) : list;
  }, [q, index]);

  // Group for display, then number entries in display order so ↑/↓ follow what's on screen.
  const { groups, ordered } = useMemo(() => {
    const map = new Map<string, SearchEntry[]>();
    for (const entry of results) map.set(entry.group, [...(map.get(entry.group) ?? []), entry]);
    const ordered: SearchEntry[] = [];
    const groups = [...map.entries()].map(
      ([group, entries]) => [group, entries.map((entry) => ({ entry, i: ordered.push(entry) - 1 }))] as const,
    );
    return { groups, ordered };
  }, [results]);

  const go = (e: SearchEntry | undefined) => {
    if (!e) return;
    onClose();
    setQ("");
    router.push(e.href);
  };

  const query = q.trim();

  return (
    <dialog
      ref={ref}
      aria-label="Search docs"
      onClose={onClose}
      onClick={(e) => e.target === ref.current && onClose()}
      className={`docs m-0 mx-auto mt-4 w-[calc(100%-32px)] max-w-[640px] overflow-hidden rounded-2xl border border-d-border bg-d-code p-0 text-d-fg backdrop:bg-black/70 backdrop:backdrop-blur-[4px] sm:mt-28`}
      data-theme={theme}
    >
      <div className="flex h-14 items-center gap-2.5 border-b border-d-border pr-3.5 pl-[18px]">
        <Search size={18} className="shrink-0 text-d-subtle" aria-hidden="true" />
        <label htmlFor={`${listId}-q`} className="sr-only">
          Search docs
        </label>
        <input
          ref={inputRef}
          id={`${listId}-q`}
          type="search"
          role="combobox"
          aria-expanded={results.length > 0}
          aria-controls={listId}
          aria-activedescendant={results.length ? `${listId}-o${sel}` : undefined}
          autoComplete="off"
          placeholder="Search docs"
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setSel(0);
          }}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") {
              e.preventDefault();
              setSel((s) => Math.min(s + 1, ordered.length - 1));
            } else if (e.key === "ArrowUp") {
              e.preventDefault();
              setSel((s) => Math.max(s - 1, 0));
            } else if (e.key === "Enter") {
              e.preventDefault();
              go(ordered[sel]);
            }
          }}
          className="h-10 grow bg-transparent text-base text-d-strong outline-none placeholder:text-d-subtle [&::-webkit-search-cancel-button]:hidden"
        />
        <kbd className="flex h-[22px] items-center rounded border border-d-border bg-d-card px-1.5 font-mono text-[11px] text-d-muted">
          Esc
        </kbd>
      </div>

      <div id={listId} role="listbox" aria-label="Results" className="flex max-h-[60dvh] flex-col gap-2 overflow-y-auto p-2">
        {results.length === 0 && (
          <p className="px-2.5 py-8 text-center text-sm text-d-subtle">No results for “{query}”. Try a different word.</p>
        )}
        {groups.map(([group, items]) => (
          <div key={group} role="group" aria-label={group} className="flex flex-col gap-0.5">
            <div className="px-2.5 pt-2 pb-1 text-xs leading-4 font-medium text-d-subtle">{group}</div>
            {items.map(({ entry, i }) => {
              const active = i === sel;
              const Icon = entry.heading ? Hash : FileText;
              return (
                <div
                  key={entry.href + i}
                  id={`${listId}-o${i}`}
                  role="option"
                  aria-selected={active}
                  onMouseMove={() => setSel(i)}
                  onClick={() => go(entry)}
                  className={`flex cursor-pointer items-center gap-3 rounded-lg p-2.5 ${active ? "bg-d-active" : ""}`}
                >
                  <Icon size={16} className="shrink-0 text-d-subtle" aria-hidden="true" />
                  <div className="min-w-0 grow">
                    <div className={`text-sm leading-5 ${active ? "text-d-strong" : "text-d-fg"}`}>
                      {entry.heading ? (
                        <>
                          {entry.page} <span className="text-d-subtle">›</span> {highlight(entry.heading, query, "")}
                        </>
                      ) : (
                        highlight(entry.page, query, "")
                      )}
                    </div>
                    <div className={`truncate text-xs leading-[18px] ${active ? "text-d-muted" : "text-d-subtle"}`}>
                      {highlight(snippet(entry.text, query), query, "text-d-icon")}
                    </div>
                  </div>
                  {active && (
                    <kbd className="flex h-[22px] items-center rounded border border-d-border2 bg-d-card px-1.5 font-mono text-[11px] text-d-icon">
                      ↵
                    </kbd>
                  )}
                </div>
              );
            })}
          </div>
        ))}
      </div>

      <div className="hidden h-11 items-center gap-5 border-t border-d-border px-[18px] text-xs text-d-subtle sm:flex">
        <span className="flex items-center gap-1.5">
          <Kbd>↑</Kbd>
          <Kbd>↓</Kbd>
          to move
        </span>
        <span className="flex items-center gap-1.5">
          <Kbd>↵</Kbd>
          to open
        </span>
        <span className="flex items-center gap-1.5">
          <Kbd>Esc</Kbd>
          to close
        </span>
      </div>
    </dialog>
  );
}

function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="flex h-5 min-w-5 items-center justify-center rounded border border-d-border bg-d-card px-[5px] font-mono text-[11px] text-d-icon">
      {children}
    </kbd>
  );
}
