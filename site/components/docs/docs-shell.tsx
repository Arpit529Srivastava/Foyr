"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState, useSyncExternalStore, type ReactNode } from "react";
import { Menu, Moon, Search, Sun, X } from "lucide-react";
import { FoyrMark } from "@/components/logo";
import { WaitlistButton } from "@/components/waitlist";
import { docsNav, docsPages, type DocLink } from "@/lib/docs-nav";
import { DocsSearch, type SearchEntry } from "@/components/docs/search";
import { Toc } from "@/components/docs/toc";
import { DocsPager } from "@/components/docs/pager";

const THEME_KEY = "foyr-docs-theme";
type Theme = "dark" | "light";

// Docs theme lives in localStorage; the server always renders dark.
const themeListeners = new Set<() => void>();
function readTheme(): Theme {
  try {
    return localStorage.getItem(THEME_KEY) === "light" ? "light" : "dark";
  } catch {
    return "dark";
  }
}
function writeTheme(t: Theme) {
  try {
    localStorage.setItem(THEME_KEY, t);
  } catch {
    /* storage unavailable: theme still changes for this page view */
  }
  memoryTheme = t;
  themeListeners.forEach((l) => l());
}
let memoryTheme: Theme | null = null;
function subscribeTheme(l: () => void) {
  themeListeners.add(l);
  window.addEventListener("storage", l);
  return () => {
    themeListeners.delete(l);
    window.removeEventListener("storage", l);
  };
}

export function DocsShell({ children, searchIndex }: { children: ReactNode; searchIndex: SearchEntry[] }) {
  const pathname = usePathname();
  const theme = useSyncExternalStore<Theme>(
    subscribeTheme,
    () => memoryTheme ?? readTheme(),
    () => "dark",
  );
  const [navOpen, setNavOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);

  const [lastPath, setLastPath] = useState(pathname);
  if (pathname !== lastPath) {
    setLastPath(pathname);
    setNavOpen(false);
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setSearchOpen((o) => !o);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const toggleTheme = () => writeTheme(theme === "dark" ? "light" : "dark");

  return (
    <div className="docs min-h-dvh" data-theme={theme}>
      <a
        href="#docs-content"
        className="sr-only z-[60] rounded-lg bg-d-strong px-3 py-2 text-sm font-medium text-d-bg focus:not-sr-only focus:fixed focus:top-3 focus:left-3"
      >
        Skip to content
      </a>
      <header className="fixed inset-x-0 top-0 z-40 flex h-[57px] items-center gap-3 border-b border-d-hair bg-d-header px-2 backdrop-blur-md sm:gap-6 sm:px-6">
        <button
          type="button"
          aria-label={navOpen ? "Close navigation" : "Open navigation"}
          aria-expanded={navOpen}
          aria-controls="docs-nav"
          onClick={() => setNavOpen((o) => !o)}
          className="flex size-11 cursor-pointer items-center justify-center rounded-lg text-d-fg lg:hidden"
        >
          {navOpen ? <X size={20} aria-hidden="true" /> : <Menu size={20} aria-hidden="true" />}
        </button>
        <div className="flex items-center gap-2.5 lg:w-48">
          <Link href="/" aria-label="Foyr home" className="flex items-center gap-[7px] text-d-strong">
            <FoyrMark />
            <span className="text-[18px] leading-5 font-[560] tracking-[-0.04em]">foyr</span>
          </Link>
          <span aria-hidden="true" className="text-base text-d-border2">
            /
          </span>
          <Link href="/docs" className="text-sm font-medium text-d-muted">
            Docs
          </Link>
        </div>
        <button
          type="button"
          aria-label="Search docs"
          aria-keyshortcuts="Meta+K"
          onClick={() => setSearchOpen(true)}
          className="ml-auto flex size-11 cursor-pointer items-center justify-center rounded-lg text-d-fg sm:ml-0 sm:h-9 sm:w-80 sm:justify-start sm:gap-2 sm:border sm:border-d-border sm:bg-d-card sm:pr-2 sm:pl-3 sm:text-sm sm:text-d-subtle"
        >
          <Search size={16} aria-hidden="true" />
          <span className="hidden grow text-left sm:inline">Search docs</span>
          <kbd className="hidden h-[22px] items-center rounded border border-d-border bg-d-bg px-1.5 font-mono text-[11px] text-d-muted sm:flex">
            ⌘K
          </kbd>
        </button>
        <div className="hidden grow sm:block" />
        <nav aria-label="Site" className="hidden items-center gap-1 md:flex">
          <Link href="/" className="flex h-8 items-center px-3 text-sm font-medium text-d-muted hover:text-d-strong">
            Home
          </Link>
          <Link href="/changelog" className="flex h-8 items-center px-3 text-sm font-medium text-d-muted hover:text-d-strong">
            Changelog
          </Link>
        </nav>
        <button
          type="button"
          onClick={toggleTheme}
          aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
          className="flex size-11 cursor-pointer items-center justify-center rounded-lg text-d-muted hover:text-d-strong sm:size-8"
        >
          {theme === "dark" ? <Sun size={16} aria-hidden="true" /> : <Moon size={16} aria-hidden="true" />}
        </button>
        <div className="hidden sm:block">
          <WaitlistButton size="sm" className="docs-cta" />
        </div>
      </header>

      <div className="mx-auto grid max-w-[1440px] pt-[57px] lg:grid-cols-[240px_minmax(0,1fr)] xl:grid-cols-[240px_minmax(0,1fr)_240px]">
        <nav
          id="docs-nav"
          aria-label="Docs"
          className={`${
            navOpen ? "fixed inset-x-0 top-[57px] bottom-0 z-30 block overflow-y-auto bg-d-bg" : "hidden"
          } border-r border-d-border px-3 py-7 lg:sticky lg:top-[57px] lg:block lg:h-[calc(100dvh-57px)] lg:overflow-y-auto`}
        >
          <div className="flex flex-col gap-6">
            {docsNav.map((group, i) => (
              <div key={group.title ?? i} className="flex flex-col gap-0.5">
                {group.title && (
                  <div className="px-2.5 pb-1.5 text-[13px] leading-5 font-medium text-d-strong">{group.title}</div>
                )}
                {group.links.map((link) => (
                  <NavLink key={link.href} link={link} active={pathname === link.href} strong={!group.title} />
                ))}
              </div>
            ))}
          </div>
        </nav>

        <main id="docs-content" className="min-w-0 px-4 pt-8 pb-24 sm:px-8 lg:pt-12">
          <article className="mx-auto max-w-[720px]">
            <Breadcrumb pathname={pathname} />
            {children}
            <DocsPager />
          </article>
        </main>

        <aside aria-label="On this page" className="hidden xl:block">
          <div className="sticky top-[57px] max-h-[calc(100dvh-57px)] overflow-y-auto pt-[52px] pr-6 pl-4">
            <Toc />
          </div>
        </aside>
      </div>

      <DocsSearch open={searchOpen} onClose={() => setSearchOpen(false)} index={searchIndex} theme={theme} />
    </div>
  );
}

function NavLink({ link, active, strong }: { link: DocLink; active: boolean; strong?: boolean }) {
  return (
    <Link
      href={link.href}
      aria-current={active ? "page" : undefined}
      className={`flex h-9 items-center rounded-md px-2.5 transition-colors lg:h-8 ${link.mono ? "font-mono text-[13px]" : strong ? "text-[13px] font-medium" : "text-sm"} ${
        active
          ? "bg-d-active font-medium text-d-strong"
          : strong
            ? "text-d-strong hover:bg-d-active"
            : "text-d-muted hover:text-d-strong"
      }`}
    >
      {link.title}
    </Link>
  );
}

function Breadcrumb({ pathname }: { pathname: string }) {
  const page = docsPages.find((p) => p.href === pathname);
  if (!page) return null;
  return (
    <nav aria-label="Breadcrumb" className="flex items-center gap-1.5 text-[13px] leading-5 text-d-subtle">
      <Link href="/docs" className="hover:text-d-fg">
        Docs
      </Link>
      {page.group && (
        <>
          <span aria-hidden="true">/</span>
          <span>{page.group}</span>
        </>
      )}
      <span aria-hidden="true">/</span>
      <span aria-current="page" className="text-d-fg">
        {page.title}
      </span>
    </nav>
  );
}
