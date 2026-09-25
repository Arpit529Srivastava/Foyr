import { ImageResponse } from "next/og";
import { OgCard, ogFonts, ogSize } from "@/lib/og";

export const alt = "Foyr — Deploy the app you already built. Login included.";
export const size = ogSize;
export const contentType = "image/png";

export default async function Image() {
  return new ImageResponse(<OgCard title="Deploy the app you already built. Login included." />, {
    ...size,
    fonts: await ogFonts(),
  });
}
