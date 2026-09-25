import type { Metadata } from "next";
import Image from "next/image";
import { SiteHeader } from "@/components/site-header";
import { ButtonLink } from "@/components/ui";
import doorway from "@/public/images/card-live.jpg";

export const metadata: Metadata = { title: "Page not found" };

export default function NotFound() {
  return (
    <>
      <SiteHeader />
      <main id="main" className="relative flex min-h-dvh items-center justify-center overflow-hidden px-4 pt-[57px]">
        <Image
          src={doorway}
          alt=""
          placeholder="blur"
          sizes="420px"
          className="absolute top-1/2 left-1/2 h-[80dvh] w-[min(420px,80vw)] -translate-x-1/2 -translate-y-1/2 rounded-2xl object-cover opacity-45"
        />
        <div aria-hidden="true" className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,#0000_0%,#000_70%)]" />
        <div className="relative flex flex-col items-center gap-4 text-center">
          <p className="font-mono text-[13px] text-fg">404</p>
          <h1 className="text-[40px] leading-[48px] font-[510] tracking-[-0.022em] text-white sm:text-[60px] sm:leading-[72px]">
            This door&apos;s locked.
          </h1>
          <p className="text-lg leading-7 text-fg/80 [text-shadow:0_1px_12px_#000]">
            The page you&apos;re looking for isn&apos;t here.
          </p>
          <div className="mt-4 flex gap-3">
            <ButtonLink href="/" arrow>
              Back home
            </ButtonLink>
            <ButtonLink href="/docs" variant="secondary">
              Read the docs
            </ButtonLink>
          </div>
        </div>
      </main>
    </>
  );
}
