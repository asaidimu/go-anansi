// Builds the publish manifest for packages/anansi/dist.
//
// `package.json` is the single source of truth. This derives
// `dist/package.json` from it (flat layout: `./index.*` instead of
// `./dist/index.*`) and copies `README.md` into `dist/`.
// Runs as the `postbuild` step (`bun run build`); writes atomically,
// no intermediate files, no shell `cp`/`mv`.
import {
  copyFileSync,
  mkdirSync,
  readFileSync,
  renameSync,
  writeFileSync,
} from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const pkgDir = join(dirname(fileURLToPath(import.meta.url)), "..");
const pkg = JSON.parse(readFileSync(join(pkgDir, "package.json"), "utf8"));
const distDir = join(pkgDir, "dist");

const distManifest = {
  name: pkg.name,
  version: pkg.version,
  description: pkg.description,
  type: "module",
  sideEffects: false,
  main: "./index.cjs",
  module: "./index.mjs",
  types: "./index.d.mts",
  exports: {
    ".": {
      types: {
        import: "./index.d.mts",
        require: "./index.d.cts",
      },
      import: "./index.mjs",
      require: "./index.cjs",
    },
    "./package.json": "./package.json",
  },
  files: ["index.mjs", "index.cjs", "index.d.mts", "index.d.cts", "README.md"],
  keywords: pkg.keywords,
  author: pkg.author,
  license: pkg.license,
  repository: pkg.repository,
  bugs: pkg.bugs,
  homepage: pkg.homepage,
  publishConfig: pkg.publishConfig,
  // fzstd/hash-wasm are imported by the bundle; @asaidimu/query and
  // @standard-schema/spec surface in the public .d.ts, so consumers need
  // them installed too.
  dependencies: pkg.dependencies ?? {},
};

mkdirSync(distDir, { recursive: true });
copyFileSync(join(pkgDir, "README.md"), join(distDir, "README.md"));
const tmp = join(distDir, ".package.json.tmp");
writeFileSync(tmp, JSON.stringify(distManifest, null, 2) + "\n");
renameSync(tmp, join(distDir, "package.json"));
console.log(`synced dist manifest → ${distManifest.name}@${distManifest.version}`);
