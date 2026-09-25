import type { NextConfig } from "next";
import createMDX from "@next/mdx";
import { foyrDark, foyrLight } from "./lib/code-themes.mjs";

const nextConfig: NextConfig = {
  pageExtensions: ["ts", "tsx", "md", "mdx"],
};

// Turbopack needs plugins by name with serializable options.
const withMDX = createMDX({
  options: {
    remarkPlugins: ["remark-gfm"],
    rehypePlugins: [
      "rehype-slug",
      ["rehype-pretty-code", { theme: { dark: foyrDark, light: foyrLight }, keepBackground: false, defaultLang: "plaintext" }],
    ],
  },
});

export default withMDX(nextConfig);
