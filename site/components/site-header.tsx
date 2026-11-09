"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { ChevronDown, Menu, X } from "lucide-react";
import { FoyrLockup } from "@/components/logo";
import { ButtonLink } from "@/components/ui";
import { WaitlistButton } from "@/components/waitlist";
import { mainNav, productMenu } from "@/lib/site";

export function SiteHeader() {
  const pathname = usePathname();
  const [menuOpen, setMenuOpen] = useState(false);
  const [productOpen, setProductOpen] = useState(false);
  const productRef = useRef<HTMLDivElement>(null);

  // Close menus on navigation.
  const [lastPath, setLastPath] = useState(pathname);
  if (pathname !== lastPath) {
    setLastPath(pathname);
    setMenuOpen(false);
    setProductOpen(false);
  }

  useEffect(() => {
    if (!productOpen) return;
    const onDown = (e: PointerEvent) => {
      if (!productRef.current?.contains(e.target as Node)) setProductOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setProductOpen(false);
    document.addEventListener("pointerdown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [productOpen]);

  useEffect(() => {
    document.body.style.overflow = menuOpen ? "hidden" : "";
    return () => {
      document.body.style.overflow = "";
    };
  }, [menuOpen]);

  const isActive = (href: string) => pathname === href || pathname.startsWith(href + "/");

  return (
    <>
      <header className="fixed inset-x-0 top-0 z-50 h-[57px] border-b border-white/8 bg-black/80 backdrop-blur-md">
        <div className="mx-auto grid h-full max-w-[1200px] grid-cols-[1fr_auto] items-center pr-2 pl-4 sm:px-6 md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
          <FoyrLockup className="justify-self-start" />

          <nav aria-label="Main" className="hidden items-center gap-1 md:flex">
            <div ref={productRef} className="relative">
              <button
                type="button"
                aria-expanded={productOpen}
                aria-controls="product-menu"
                onClick={() => setProductOpen((o) => !o)}
                className="flex h-8 cursor-pointer items-center gap-1 rounded-md px-3 text-sm font-medium text-dim transition-colors hover:text-fg"
              >
                Product
                <ChevronDown
                  size={14}
                  className={`transition-transform duration-200 ${productOpen ? "rotate-180" : ""}`}
                  aria-hidden="true"
                />
              </button>
              {productOpen && (
                <div
                  id="product-menu"
                  className="absolute top-[calc(100%+12px)] left-1/2 w-[420px] -translate-x-1/2 animate-[foyr-menu_200ms_var(--ease-out-soft)_both] rounded-xl border border-line bg-panel p-2 shadow-[0_16px_48px_#000c]"
                >
                  <ul className="grid gap-1">
                    {productMenu.map((item) => (
                      <li key={item.label}>
                        <Link href={item.href} className="block rounded-lg px-3 py-2.5 transition-colors hover:bg-raised">
                          <span className="block text-sm font-medium text-white">{item.label}</span>
                          <span className="mt-0.5 block text-[13px] leading-5 text-muted">{item.description}</span>
                        </Link>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
            {mainNav.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                aria-current={isActive(item.href) ? "page" : undefined}
                className={`flex h-8 items-center rounded-md px-3 text-sm font-medium transition-colors hover:text-fg ${
                  isActive(item.href) ? "text-white" : "text-dim"
                }`}
              >
                {item.label}
              </Link>
            ))}
          </nav>

          <div className="hidden justify-end gap-2 md:flex">
            <ButtonLink href="/docs" variant="secondary" size="sm">
              Read the docs
            </ButtonLink>
            <WaitlistButton size="sm" />
          </div>

          <button
            type="button"
            aria-label={menuOpen ? "Close menu" : "Open menu"}
            aria-expanded={menuOpen}
            aria-controls="mobile-menu"
            onClick={() => setMenuOpen((o) => !o)}
            className="flex size-11 cursor-pointer items-center justify-center justify-self-end rounded-lg text-fg md:hidden"
          >
            {menuOpen ? <X size={20} aria-hidden="true" /> : <Menu size={20} aria-hidden="true" />}
          </button>
        </div>
      </header>

      {/* Outside the header: its backdrop-blur would make it the containing block for this fixed panel. */}
      {menuOpen && (
        <div
          id="mobile-menu"
          className="fixed inset-x-0 top-[57px] bottom-0 z-50 animate-fade overflow-y-auto bg-black px-4 pt-4 pb-10 md:hidden"
        >
          <div className="text-xs font-medium tracking-[0.08em] text-subtle uppercase">Product</div>
          <ul className="mt-2 border-b border-line pb-4">
            {productMenu.map((item) => (
              <li key={item.label}>
                <Link href={item.href} className="flex min-h-12 flex-col justify-center py-1.5">
                  <span className="text-base font-medium text-white">{item.label}</span>
                  <span className="text-[13px] text-muted">{item.description}</span>
                </Link>
              </li>
            ))}
          </ul>
          <ul className="mt-2">
            {mainNav.map((item) => (
              <li key={item.href}>
                <Link href={item.href} className="flex h-12 items-center text-base font-medium text-fg">
                  {item.label}
                </Link>
              </li>
            ))}
          </ul>
          <div className="mt-6 grid grid-cols-2 gap-3">
            <ButtonLink href="/docs" variant="secondary" size="lg">
              Read the docs
            </ButtonLink>
            <WaitlistButton size="lg" />
          </div>
        </div>
      )}
    </>
  );
}
