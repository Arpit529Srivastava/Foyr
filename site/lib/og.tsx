import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { siteUrl } from "@/lib/site";

export const ogSize = { width: 1200, height: 630 };

const fontDir = join(process.cwd(), "node_modules/@fontsource/inter/files");

export async function ogFonts() {
  const [medium, semibold] = await Promise.all([
    readFile(join(fontDir, "inter-latin-500-normal.woff")),
    readFile(join(fontDir, "inter-latin-600-normal.woff")),
  ]);
  return [
    { name: "Inter", data: medium, weight: 500 as const, style: "normal" as const },
    { name: "Inter", data: semibold, weight: 600 as const, style: "normal" as const },
  ];
}

export function Mark({ size }: { size: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="#ffffff" shapeRendering="crispEdges">
      <rect x="2" y="1" width="12" height="2" />
      <rect x="2" y="3" width="2" height="12" />
      <rect x="12" y="3" width="2" height="12" />
      <rect x="5" y="4" width="4" height="11" />
    </svg>
  );
}

/** Social card from the design's OgImage board: doorway light on the right, logo + headline on the left. */
export async function OgCard({ title, eyebrow }: { title: string; eyebrow?: string }) {
  const photo = await readFile(join(process.cwd(), "public/images/doorway-hero.jpg"), "base64");
  const host = new URL(siteUrl).host;
  return (
    <div
      style={{
        position: "relative",
        width: "100%",
        height: "100%",
        display: "flex",
        background: "#000",
        color: "#fff",
        fontFamily: "Inter",
      }}
    >
      {/* eslint-disable-next-line @next/next/no-img-element -- rendered by ImageResponse, not the browser */}
      <img
        src={`data:image/jpeg;base64,${photo}`}
        alt=""
        width={640}
        height={630}
        style={{
          position: "absolute",
          right: 0,
          top: 0,
          width: 640,
          height: 630,
          objectFit: "cover",
          objectPosition: "70% center",
        }}
      />
      <div
        style={{
          position: "absolute",
          right: 0,
          top: 0,
          width: 640,
          height: 630,
          backgroundImage: "linear-gradient(90deg, #000000 0%, rgba(0,0,0,0) 45%)",
        }}
      />
      <div
        style={{
          position: "relative",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          width: 700,
          height: "100%",
          padding: "72px 80px",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
          <Mark size={32} />
          <span style={{ fontSize: 36, fontWeight: 600, letterSpacing: "-0.04em" }}>foyr</span>
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
          {eyebrow && (
            <div style={{ fontSize: 20, color: "#8a8a8a", letterSpacing: "0.08em", textTransform: "uppercase" }}>{eyebrow}</div>
          )}
          <div style={{ fontSize: 60, lineHeight: 1.13, fontWeight: 500, letterSpacing: "-0.022em" }}>{title}</div>
          <div style={{ fontSize: 20, color: "#a3a3a3" }}>{host}</div>
        </div>
      </div>
    </div>
  );
}
