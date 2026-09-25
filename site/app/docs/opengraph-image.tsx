import { ImageResponse } from "next/og";
import { OgCard, ogFonts, ogSize } from "@/lib/og";

export const alt = "Foyr Docs";
export const size = ogSize;
export const contentType = "image/png";

export default async function Image() {
  return new ImageResponse(<OgCard eyebrow="Docs" title="How Foyr deploys, logs in and gates your app." />, {
    ...size,
    fonts: await ogFonts(),
  });
}
