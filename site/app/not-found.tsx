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
      <main id="main" className="flex min-h-dvh flex-col items-center justify-center gap-10 px-4 pt-[57px] pb-10 text-center">
        {/* The doorway from the design: an arched opening with the warm light behind it. */}
        <div className="relative h-[280px] w-[176px] overflow-hidden rounded-t-[88px] rounded-b-xl shadow-[0_0_0_1px_#ffffff14] sm:h-[320px] sm:w-[200px] sm:rounded-t-[100px]">
          <Image
            src={doorway}
            alt=""
            priority
            placeholder="blur"
            sizes="200px"
            className="size-full object-cover object-[50%_40%]"
          />
        </div>
        <div className="flex flex-col items-center gap-4">
          <h1 className="text-[40px] leading-[48px] font-[510] tracking-[-0.022em] text-white sm:text-[60px] sm:leading-[72px]">
            This door&apos;s locked.
          </h1>
          <p className="text-lg leading-7 text-muted">The page you&apos;re looking for isn&apos;t here.</p>
        </div>
        <div className="flex gap-3">
          <ButtonLink href="/" arrow>
            Back home
          </ButtonLink>
          <ButtonLink href="/docs" variant="secondary">
            Read the docs
          </ButtonLink>
        </div>
      </main>
    </>
  );
}
