// Prepares self-hosted assets for the developer docs portal (/docs).
// Runs as `prebuild` so the standalone build always ships them:
//   1. Copies Scalar's standalone browser bundle from node_modules into
//      public/vendor/ (served same-origin — no external CDN in production).
//   2. Syncs the OpenAPI contracts from contracts/openapi/ (source of truth)
//      into public/openapi/ when the monorepo is present. The copies in
//      public/openapi/ are committed so the Docker build (whose context only
//      copies apps/console/) still works without the contracts directory.
import { copyFileSync, existsSync, mkdirSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(dirname(fileURLToPath(import.meta.url))); // apps/console

// 1. Scalar standalone bundle
const scalarSrc = join(
  root,
  "node_modules/@scalar/api-reference/dist/browser/standalone.js",
);
const vendorDir = join(root, "public/vendor");
mkdirSync(vendorDir, { recursive: true });
copyFileSync(scalarSrc, join(vendorDir, "scalar.standalone.js"));
console.log("[docs-assets] copied scalar.standalone.js -> public/vendor/");

// 2. OpenAPI contracts (monorepo source of truth -> committed copies)
const contractsDir = join(root, "../../contracts/openapi");
const specDir = join(root, "public/openapi");
mkdirSync(specDir, { recursive: true });
if (existsSync(contractsDir)) {
  for (const f of readdirSync(contractsDir).filter((f) => f.endsWith(".yaml"))) {
    copyFileSync(join(contractsDir, f), join(specDir, f));
  }
  console.log("[docs-assets] synced contracts/openapi -> public/openapi/");
} else {
  console.log(
    "[docs-assets] contracts/openapi not found; using committed copies in public/openapi/",
  );
}
