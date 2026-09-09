// Syncs the publish manifest (dist.package.json) with the workspace
// package.json: version + runtime dependencies flow into dist so the npm
// plugin can publish `dist/` as a self-contained package root.
import { readFileSync, writeFileSync } from "fs";

function updateDistPackage(): void {
  const pkg = JSON.parse(readFileSync("package.json", "utf8"));
  const dist = JSON.parse(readFileSync("dist.package.json", "utf8"));

  dist.version = pkg.version;
  dist.dependencies = pkg.dependencies || {};

  writeFileSync("out.package.json", JSON.stringify(dist, null, 2) + "\n");
  console.log(`synced dist manifest → @asaidimu/anansi@${pkg.version}`);
}

updateDistPackage();
