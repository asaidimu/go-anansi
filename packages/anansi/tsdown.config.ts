import { defineConfig } from "tsdown";

export default defineConfig({
  entry: {
    index: "src/index.ts",
  },
  outDir: "dist",
  format: ["esm", "cjs"],
  unbundle: false,
  minify: true,
  clean: true,
  sourcemap: false,
  dts: true,
  // Anansi packets are plain Uint8Array/JSON values — no Node built-ins in
  // the codec paths, so the bundle is safe for browsers and workers.
  platform: "neutral",
  fixedExtension: true,
});
