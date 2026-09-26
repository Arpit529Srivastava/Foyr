import type { Metadata, Viewport } from "next";
import { Geist_Mono, Inter } from "next/font/google";
import { WaitlistProvider } from "@/components/waitlist";
import { isIndexable, site, siteUrl } from "@/lib/site";
import "./globals.css";

const inter = Inter({ variable: "--font-inter", subsets: ["latin"], display: "swap" });
const geistMono = Geist_Mono({ variable: "--font-geist-mono", subsets: ["latin"], display: "swap" });

export const metadata: Metadata = {
  metadataBase: new URL(siteUrl),
  robots: isIndexable ? undefined : { index: false, follow: false },
  title: { default: site.title, template: "%s · Foyr" },
  description: site.description,
  applicationName: "Foyr",
  openGraph: {
    type: "website",
    siteName: "Foyr",
    title: site.title,
    description: site.description,
    url: "/",
  },
  twitter: { card: "summary_large_image", title: site.title, description: site.description },
  alternates: { canonical: "/" },
};

export const viewport: Viewport = {
  themeColor: "#000000",
  colorScheme: "dark",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" data-scroll-behavior="smooth" className={`${inter.variable} ${geistMono.variable}`}>
      <body className="min-h-dvh">
        <WaitlistProvider>{children}</WaitlistProvider>
      </body>
    </html>
  );
}
