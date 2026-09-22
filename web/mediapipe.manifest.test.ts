import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

import manifest from "./mediapipe.manifest.json";

const root = dirname(fileURLToPath(import.meta.url));

describe("mediapipe pin", () => {
  it("pins exact package version and model sha256", () => {
    expect(manifest.packageVersion).toBe("1.0.1");
    expect(manifest.modelPath).not.toMatch(/@latest/);
    expect(manifest.wasmPath).not.toMatch(/@latest/);
    if ("modelSourceUrl" in manifest && typeof manifest.modelSourceUrl === "string") {
      expect(manifest.modelSourceUrl).not.toMatch(/@latest/);
    }

    const bytes = readFileSync(
      join(root, "public", "mediapipe", "models", "face_landmarker.task"),
    );
    const hash = createHash("sha256").update(bytes).digest("hex");
    expect(hash).toBe(manifest.modelSha256);
  });
});
