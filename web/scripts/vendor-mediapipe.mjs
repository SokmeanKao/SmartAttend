/**
 * Vendor MediaPipe Face Landmarker assets for same-origin serving.
 *
 * Model download URL (pinned at vendor time by SHA256 of bytes — not @latest):
 * https://storage.googleapis.com/mediapipe-models/face_landmarker/face_landmarker/float16/1/face_landmarker.task
 *
 * Usage:
 *   node scripts/vendor-mediapipe.mjs
 *   node scripts/vendor-mediapipe.mjs --verify-only
 */

import { createHash } from "node:crypto";
import {
  copyFileSync,
  createWriteStream,
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  writeFileSync,
} from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { pipeline } from "node:stream/promises";
import { Readable } from "node:stream";

const __dirname = dirname(fileURLToPath(import.meta.url));
const WEB_ROOT = join(__dirname, "..");
const REQUIRED_PACKAGE_VERSION = "1.0.1";

/** @see comment at top of file — float16 Face Landmarker bundle path segment "1". */
const MODEL_URL =
  "https://storage.googleapis.com/mediapipe-models/face_landmarker/face_landmarker/float16/1/face_landmarker.task";

const PACKAGE_DIR = join(WEB_ROOT, "node_modules", "@mediapipe", "tasks-vision");
const WASM_SRC = join(PACKAGE_DIR, "wasm");
const WASM_DEST = join(WEB_ROOT, "public", "mediapipe", "wasm");
const MODEL_DEST = join(
  WEB_ROOT,
  "public",
  "mediapipe",
  "models",
  "face_landmarker.task",
);
const MANIFEST_PATH = join(WEB_ROOT, "mediapipe.manifest.json");

const verifyOnly = process.argv.includes("--verify-only");

function fail(message) {
  console.error(`mediapipe:vendor: ${message}`);
  process.exit(1);
}

function assertNoLatest(value, label) {
  if (typeof value === "string" && value.includes("@latest")) {
    fail(`${label} must not contain @latest: ${value}`);
  }
}

function readInstalledVersion() {
  const pkgPath = join(PACKAGE_DIR, "package.json");
  if (!existsSync(pkgPath)) {
    fail(`@mediapipe/tasks-vision not installed at ${pkgPath}`);
  }
  const pkg = JSON.parse(readFileSync(pkgPath, "utf8"));
  return pkg.version;
}

function sha256File(path) {
  const bytes = readFileSync(path);
  return createHash("sha256").update(bytes).digest("hex");
}

function copyWasm() {
  if (!existsSync(WASM_SRC)) {
    fail(`WASM source missing: ${WASM_SRC}`);
  }
  mkdirSync(WASM_DEST, { recursive: true });
  for (const name of readdirSync(WASM_SRC)) {
    copyFileSync(join(WASM_SRC, name), join(WASM_DEST, name));
  }
}

async function downloadModel() {
  mkdirSync(dirname(MODEL_DEST), { recursive: true });
  if (existsSync(MODEL_DEST) && !process.argv.includes("--force")) {
    console.log(`Model already present: ${MODEL_DEST}`);
    return;
  }
  console.log(`Downloading model from ${MODEL_URL}`);
  const response = await fetch(MODEL_URL);
  if (!response.ok || !response.body) {
    fail(`Model download failed: HTTP ${response.status}`);
  }
  await pipeline(Readable.fromWeb(response.body), createWriteStream(MODEL_DEST));
}

function writeManifest(modelSha256) {
  const manifest = {
    packageVersion: REQUIRED_PACKAGE_VERSION,
    modelPath: "/mediapipe/models/face_landmarker.task",
    wasmPath: "/mediapipe/wasm",
    modelSha256,
    modelSourceUrl: MODEL_URL,
  };
  assertNoLatest(manifest.modelPath, "modelPath");
  assertNoLatest(manifest.wasmPath, "wasmPath");
  assertNoLatest(manifest.modelSourceUrl, "modelSourceUrl");
  writeFileSync(MANIFEST_PATH, `${JSON.stringify(manifest, null, 2)}\n`);
  return manifest;
}

function verify() {
  const version = readInstalledVersion();
  if (version !== REQUIRED_PACKAGE_VERSION) {
    fail(
      `Installed @mediapipe/tasks-vision is ${version}, required ${REQUIRED_PACKAGE_VERSION}`,
    );
  }
  if (!existsSync(MANIFEST_PATH)) {
    fail(`Missing manifest: ${MANIFEST_PATH}`);
  }
  if (!existsSync(MODEL_DEST)) {
    fail(`Missing model: ${MODEL_DEST}`);
  }
  if (!existsSync(WASM_DEST)) {
    fail(`Missing wasm dir: ${WASM_DEST}`);
  }

  const manifest = JSON.parse(readFileSync(MANIFEST_PATH, "utf8"));
  assertNoLatest(manifest.modelPath, "manifest.modelPath");
  assertNoLatest(manifest.wasmPath, "manifest.wasmPath");
  if (manifest.modelSourceUrl) {
    assertNoLatest(manifest.modelSourceUrl, "manifest.modelSourceUrl");
  }
  if (manifest.packageVersion !== REQUIRED_PACKAGE_VERSION) {
    fail(
      `Manifest packageVersion ${manifest.packageVersion} !== ${REQUIRED_PACKAGE_VERSION}`,
    );
  }
  const hash = sha256File(MODEL_DEST);
  if (hash !== manifest.modelSha256) {
    fail(
      `Model SHA256 mismatch.\n  expected: ${manifest.modelSha256}\n  actual:   ${hash}`,
    );
  }
  console.log("mediapipe:verify OK", {
    packageVersion: version,
    modelSha256: hash,
  });
}

async function vendor() {
  const version = readInstalledVersion();
  if (version !== REQUIRED_PACKAGE_VERSION) {
    fail(
      `Installed @mediapipe/tasks-vision is ${version}, required ${REQUIRED_PACKAGE_VERSION}`,
    );
  }
  copyWasm();
  await downloadModel();
  const modelSha256 = sha256File(MODEL_DEST);
  const manifest = writeManifest(modelSha256);
  console.log("mediapipe:vendor OK", manifest);
}

if (verifyOnly) {
  verify();
} else {
  await vendor();
  verify();
}
