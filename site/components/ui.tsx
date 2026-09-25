import Link from "next/link";
import type { ComponentProps, ReactNode } from "react";
import { ArrowRight } from "lucide-react";

type Size = "sm" | "md" | "lg";

const sizes: Record<Size, string> = {
  sm: "h-8 px-3 text-sm",
  md: "h-10 px-4 text-sm",
  lg: "h-11 px-4 text-sm",
};

const base =
  "inline-flex items-center justify-center gap-2 rounded-lg font-medium whitespace-nowrap transition-colors duration-150 ease-[var(--ease-std)]";

export const buttonStyles = {
  primary: (size: Size = "md") => `${base} ${sizes[size]} f-arrow bg-fg text-black hover:bg-white`,
  secondary: (size: Size = "md") =>
    `${base} ${sizes[size]} border border-line bg-panel text-fg hover:border-line-strong hover:text-white`,
};

export function ButtonLink({
  variant = "primary",
  size = "md",
  arrow,
  className = "",
  children,
  ...props
}: ComponentProps<typeof Link> & { variant?: "primary" | "secondary"; size?: Size; arrow?: boolean }) {
  return (
    <Link className={`${buttonStyles[variant](size)} ${className}`} {...props}>
      {children}
      {arrow && <ArrowRight size={size === "sm" ? 14 : 16} strokeWidth={2} aria-hidden="true" />}
    </Link>
  );
}

export function Eyebrow({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={`inline-flex h-6 items-center self-start rounded border border-line px-2 text-xs font-medium tracking-[0.08em] text-subtle uppercase ${className}`}
    >
      {children}
    </div>
  );
}

export function Container({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <div className={`mx-auto w-full max-w-[1200px] px-4 sm:px-6 ${className}`}>{children}</div>;
}
