import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: {
    alias: [
      { find: /^@\//, replacement: fileURLToPath(new URL("./", import.meta.url)) },
      { find: "server-only", replacement: fileURLToPath(new URL("./test/empty.ts", import.meta.url)) },
    ],
  },
  test: {
    environment: "node",
    include: ["{app,components,lib}/**/*.test.ts"],
  },
});
