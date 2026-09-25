"use client";

import { useLayoutEffect, useRef, useState, type ReactNode } from "react";

/**
 * Renders a fixed-size product mockup (designed at width × height px) scaled down to fit its
 * container. Mockups are decorative, so they're hidden from assistive tech and not focusable.
 */
export function ScaledFrame({
  width,
  height,
  children,
  className = "",
  label,
}: {
  width: number;
  height: number;
  children: ReactNode;
  className?: string;
  label?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(1);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const update = () => setScale(Math.min(1, el.clientWidth / width));
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, [width]);

  return (
    <div
      ref={ref}
      role={label ? "img" : undefined}
      aria-label={label}
      className={`relative w-full ${className}`}
      style={{ height: height * scale, maxWidth: width }}
    >
      <div
        aria-hidden="true"
        inert
        className="absolute top-0 left-0 origin-top-left select-none"
        style={{ width, height, transform: `scale(${scale})` }}
      >
        {children}
      </div>
    </div>
  );
}
