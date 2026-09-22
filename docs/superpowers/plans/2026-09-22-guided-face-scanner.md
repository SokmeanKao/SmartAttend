# Guided Face Scanner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the button-wizard enrollment/verify camera UX with a shared oval `FaceScanner` that uses checksum-pinned MediaPipe Face Landmarker for advisory guidance and the existing Go/YuNet/SFace APIs for authoritative acceptance.

**Architecture:** Pure guidance heuristics → MediaPipe adapter → `FaceScanner` (camera + capture guard + overlay) → `EnrollmentFlow` / verify page (business + `scannerActive`). Backend Tasks 6/9 unchanged.

**Tech Stack:** Next.js 16 (web), React 19, `@mediapipe/tasks-vision@1.0.1` (exact), Vitest for web unit tests, existing Go enroll/verify APIs.

**Spec:** `docs/superpowers/specs/2026-09-22-guided-face-scanner-design.md`

## Global Constraints

- No Face ID / TrueDepth / liveness / virtual-camera security claims in UI copy
- MediaPipe is advisory only (FACE-GUIDE-02); YuNet/SFace authoritative
- No continuous image upload (FACE-GUIDE-03)
- Exact npm pin (no `^`/`~`); vendored WASM + `.task`; SHA256 assert (FACE-GUIDE-05)
- Guidance math in unmirrored sensor coordinates; `neg yaw = user's LEFT`
- Adaptive cadence target 10–15 FPS; never overlap inference; not a universal DoD gate
- Verify: `scannerActive=false` after settled match/no-match (no auto-recapture loop)
- Manual Capture: bypass readiness, not IDLE/in-flight/camera-live guards
- Do not modify Go enroll/verify contracts or face-service pose thresholds for this plan

## File structure

```text
web/
  package.json                          # add vitest + @mediapipe/tasks-vision@1.0.1 exact
  public/mediapipe/
    wasm/                               # copied from package
    models/face_landmarker.task
  mediapipe.manifest.json               # { packageVersion, modelSha256, modelPath, wasmPath }
  scripts/vendor-mediapipe.mjs          # copy wasm/model + write/verify hash
  components/face-scanner/
    types.ts
    FaceScanner.tsx
    camera/useCameraStream.ts           # lift/adapt from CameraCapture
    guidance/
      geometry.ts                       # oval, distance, centered
      yaw.ts                            # yaw proxy + hysteresis + bands
      stability.ts
      mediapipeLandmarker.ts
      mapLandmarksToGuidance.ts
      runGuidanceLoop.ts
    overlay/OvalOverlay.tsx
    overlay/GuidanceBanner.tsx
  components/enrollment/EnrollmentFlow.tsx
  app/(admin)/employees/[id]/face/page.tsx   # thin wrapper → EnrollmentFlow
  app/(public)/verify/page.tsx                # FaceScanner + scannerActive
  components/camera/CameraCapture.tsx         # delete or re-export shim after cutover
  features/enrollment/api.ts                  # unchanged API helpers
```

---

### Task 1: Vitest + MediaPipe vendor pin (FACE-GUIDE-05)

**Files:**
- Create: `web/vitest.config.ts`
- Create: `web/scripts/vendor-mediapipe.mjs`
- Create: `web/mediapipe.manifest.json`
- Create: `web/public/mediapipe/wasm/**` (generated)
- Create: `web/public/mediapipe/models/face_landmarker.task` (generated)
- Create: `web/mediapipe.manifest.test.ts`
- Modify: `web/package.json` (exact dep + scripts)
- Modify: `web/.gitignore` only if large assets should stay tracked — **track** model+wasm in repo per spec (same-origin offline)

**Interfaces:**
- Produces: `mediapipe.manifest.json` shape `{ "packageVersion": "1.0.1", "modelPath": "/mediapipe/models/face_landmarker.task", "wasmPath": "/mediapipe/wasm", "modelSha256": "<hex>" }`
- Produces: `npm run mediapipe:vendor` and `npm test` in `web/`

- [ ] **Step 1: Add exact dependency and Vitest**

In `web/package.json`:

```json
"dependencies": {
  "@mediapipe/tasks-vision": "1.0.1"
},
"devDependencies": {
  "vitest": "^3.0.0"
},
"scripts": {
  "test": "vitest run",
  "test:watch": "vitest",
  "mediapipe:vendor": "node scripts/vendor-mediapipe.mjs",
  "mediapipe:verify": "node scripts/vendor-mediapipe.mjs --verify-only"
}
```

No `^` / `~` on `@mediapipe/tasks-vision`.

- [ ] **Step 2: Write vendor script**

`web/scripts/vendor-mediapipe.mjs` must:

1. Resolve `node_modules/@mediapipe/tasks-vision/package.json` version; fail if ≠ `1.0.1`
2. Copy WASM files from the package’s `wasm/` directory to `public/mediapipe/wasm/`
3. Download or copy Face Landmarker float16 `.task` into `public/mediapipe/models/face_landmarker.task` (pin URL used at vendor time; never `@latest` at runtime)
4. Compute SHA256 of the `.task` bytes
5. Write `mediapipe.manifest.json`
6. With `--verify-only`: re-hash file and exit 1 on mismatch; also fail if any runtime path string contains `@latest`

Document the model download URL used inside the script comment.

- [ ] **Step 3: Write failing verify test**

```ts
// web/mediapipe.manifest.test.ts
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import manifest from "./mediapipe.manifest.json";

describe("mediapipe pin", () => {
  it("pins exact package version and model sha256", () => {
    expect(manifest.packageVersion).toBe("1.0.1");
    expect(manifest.modelPath).not.toMatch(/@latest/);
    expect(manifest.wasmPath).not.toMatch(/@latest/);
    const bytes = readFileSync(
      join(__dirname, "public", "mediapipe", "models", "face_landmarker.task"),
    );
    const hash = createHash("sha256").update(bytes).digest("hex");
    expect(hash).toBe(manifest.modelSha256);
  });
});
```

- [ ] **Step 4: Run vendor then tests**

```bash
cd web
npm install
npm run mediapipe:vendor
npm test
```

Expected: PASS (hash matches). Mutate one byte of the `.task` and re-run → FAIL.

- [ ] **Step 5: Commit**

```bash
git add web/package.json web/package-lock.json web/vitest.config.ts \
  web/scripts/vendor-mediapipe.mjs web/mediapipe.manifest.json \
  web/mediapipe.manifest.test.ts web/public/mediapipe
git commit -m "chore(web): pin MediaPipe tasks-vision 1.0.1 with SHA256 model"
```

---

### Task 2: Pure guidance heuristics (geometry, yaw, stability)

**Files:**
- Create: `web/components/face-scanner/types.ts`
- Create: `web/components/face-scanner/guidance/geometry.ts`
- Create: `web/components/face-scanner/guidance/yaw.ts`
- Create: `web/components/face-scanner/guidance/stability.ts`
- Create: `web/components/face-scanner/guidance/geometry.test.ts`
- Create: `web/components/face-scanner/guidance/yaw.test.ts`
- Create: `web/components/face-scanner/guidance/stability.test.ts`

**Interfaces:**
- Produces types:

```ts
export type GuidancePose = "LEFT" | "FRONT" | "RIGHT" | "UNKNOWN";
export type EnrollmentPose = "LEFT" | "FRONT" | "RIGHT";
export type FaceGuidance = {
  faceDetected: boolean;
  centered: boolean;
  distance: "TOO_CLOSE" | "GOOD" | "TOO_FAR";
  pose: GuidancePose;
  stable: boolean;
};
export type CaptureCandidate = {
  blob: Blob;
  source: "AUTO" | "MANUAL";
  guidance: FaceGuidance;
};
export type CaptureResult =
  | { status: "ACCEPTED" }
  | { status: "REJECTED" };
```

- Produces:

```ts
// geometry.ts — all coords in unmirrored source pixels
export function ovalForFrame(w: number, h: number): { cx: number; cy: number; rx: number; ry: number }
export function faceMetricsFromLandmarks(/* normalized landmarks + frame size */): { centerX: number; centerY: number; faceHeight: number }
export function isCentered(face: { centerX: number; centerY: number }, oval: { cx: number; cy: number; rx: number; ry: number }, innerScale?: number): boolean
export function classifyDistance(faceHeight: number, ovalHeight: number): "TOO_CLOSE" | "GOOD" | "TOO_FAR"

// yaw.ts — negative = user's LEFT (locked)
export function yawProxyFromLandmarks(/* … */): number
export function classifyYaw(yaw: number, prev: GuidancePose, opts?: { frontMax: number; sideMin: number; epsilon: number }): GuidancePose

// stability.ts
export function updateStability(args: {
  ready: boolean;
  nowMs: number;
  stableSince: number | null;
  windowMs?: number;
}): { stable: boolean; stableSince: number | null }
```

- [ ] **Step 1: Write failing tests for distance + centered**

```ts
import { describe, expect, it } from "vitest";
import { classifyDistance, isCentered, ovalForFrame } from "./geometry";

it("classifies distance bands", () => {
  expect(classifyDistance(0.4 * 100, 100)).toBe("TOO_FAR");
  expect(classifyDistance(0.7 * 100, 100)).toBe("GOOD");
  expect(classifyDistance(1.1 * 100, 100)).toBe("TOO_CLOSE");
});

it("centered uses inner oval", () => {
  const oval = ovalForFrame(640, 480);
  expect(isCentered({ centerX: oval.cx, centerY: oval.cy }, oval)).toBe(true);
  expect(isCentered({ centerX: 0, centerY: 0 }, oval)).toBe(false);
});
```

- [ ] **Step 2: Write failing tests for yaw + hysteresis + LEFT sign**

```ts
it("negative yaw is LEFT", () => {
  expect(classifyYaw(-0.4, "UNKNOWN", { frontMax: 0.15, sideMin: 0.25, epsilon: 0.05 })).toBe("LEFT");
  expect(classifyYaw(0.4, "UNKNOWN", { frontMax: 0.15, sideMin: 0.25, epsilon: 0.05 })).toBe("RIGHT");
});

it("hysteresis avoids chatter near threshold", () => {
  const left = classifyYaw(-0.3, "LEFT", { frontMax: 0.15, sideMin: 0.25, epsilon: 0.05 });
  expect(left).toBe("LEFT");
});
```

- [ ] **Step 3: Write failing stability tests**

```ts
it("becomes stable after window and resets when not ready", () => {
  let s = updateStability({ ready: true, nowMs: 0, stableSince: null, windowMs: 400 });
  expect(s.stable).toBe(false);
  s = updateStability({ ready: true, nowMs: 400, stableSince: s.stableSince, windowMs: 400 });
  expect(s.stable).toBe(true);
  s = updateStability({ ready: false, nowMs: 450, stableSince: s.stableSince, windowMs: 400 });
  expect(s.stable).toBe(false);
  expect(s.stableSince).toBeNull();
});
```

- [ ] **Step 4: Implement geometry/yaw/stability until `npm test` passes**

Use defaults: `frontMax=0.15`, `sideMin=0.25`, `epsilon=0.05`, stability `400` (plan may tune later; do not equal YuNet).

- [ ] **Step 5: Commit**

```bash
git add web/components/face-scanner
git commit -m "feat(web): face guidance geometry yaw and stability helpers"
```

---

### Task 3: `FaceScanner` capture state machine (no MediaPipe yet)

**Files:**
- Create: `web/components/face-scanner/camera/useCameraStream.ts` (adapt from `CameraCapture.tsx`)
- Create: `web/components/face-scanner/FaceScanner.tsx`
- Create: `web/components/face-scanner/FaceScanner.capture.test.ts` (logic extracted if needed)
- Create: `web/components/face-scanner/captureCoordinator.ts`

**Interfaces:**
- Consumes: types from Task 2; JPEG helpers from existing `CameraCapture` patterns
- Produces:

```ts
export type ScannerCaptureState = "IDLE" | "CAPTURING" | "SUBMITTING" | "COOLDOWN";

export function createCaptureCoordinator(opts?: { cooldownMs?: number }): {
  getState(): ScannerCaptureState;
  canStartCapture(): boolean;
  beginCapture(): boolean;          // false if not IDLE
  markSubmitting(): void;
  settle(result: CaptureResult): void; // → COOLDOWN then IDLE after timer
  // test hook: flushCooldown()
}

type FaceScannerProps = {
  requiredPose: EnrollmentPose;
  active?: boolean;
  onCapture: (candidate: CaptureCandidate) => Promise<CaptureResult>;
  instructionSlot?: React.ReactNode;
  progressSlot?: React.ReactNode;
  autoStart?: boolean;
};
```

- [ ] **Step 1: Failing tests — in-flight guard**

```ts
it("rejects second beginCapture while SUBMITTING", () => {
  const c = createCaptureCoordinator({ cooldownMs: 0 });
  expect(c.beginCapture()).toBe(true);
  c.markSubmitting();
  expect(c.beginCapture()).toBe(false);
});

it("returns to IDLE after settle + cooldown", async () => {
  const c = createCaptureCoordinator({ cooldownMs: 10 });
  c.beginCapture();
  c.markSubmitting();
  c.settle({ status: "ACCEPTED" });
  await new Promise((r) => setTimeout(r, 20));
  expect(c.getState()).toBe("IDLE");
});
```

- [ ] **Step 2: Implement coordinator + FaceScanner shell**

`FaceScanner` responsibilities for this task:

- `useCameraStream` (prefer `facingMode: "user"`; device `<select>` if multiple; **no** physical/virtual security filtering)
- Video element + CSS mirror for preview only
- Manual button enabled iff `active !== false` && camera live && state === `IDLE`
- Manual capture: grab frame → JPEG → `onCapture` with `source: "MANUAL"` and a stub `guidance` (or last known)
- Auto path stubbed: expose `tryAutoCapture(guidance)` for Task 4
- When `active === false`: pause auto attempts; do not dispose camera unless unmount/stop

- [ ] **Step 3: `npm test` passes**

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(web): FaceScanner capture coordinator and camera shell"
```

---

### Task 4: MediaPipe landmarker + adaptive guidance loop

**Files:**
- Create: `web/components/face-scanner/guidance/mediapipeLandmarker.ts`
- Create: `web/components/face-scanner/guidance/mapLandmarksToGuidance.ts`
- Create: `web/components/face-scanner/guidance/runGuidanceLoop.ts`
- Create: `web/components/face-scanner/guidance/runGuidanceLoop.test.ts`
- Modify: `web/components/face-scanner/FaceScanner.tsx`

**Interfaces:**
- Consumes: manifest paths; Task 2 heuristics
- Produces:

```ts
export async function createFaceLandmarker(opts: {
  wasmPath: string;
  modelPath: string;
}): Promise<{ detectForVideo: (video: HTMLVideoElement, ts: number) => unknown; close: () => void }>
// GPU first, CPU retry; throw if both fail

export function mapLandmarksToGuidance(input: {
  landmarks: Array<{ x: number; y: number; z?: number }>;
  frameW: number;
  frameH: number;
  requiredPose: EnrollmentPose;
  prevPose: GuidancePose;
  nowMs: number;
  stableSince: number | null;
}): { guidance: FaceGuidance; nextStableSince: number | null; nextPrevPose: GuidancePose }

export function createGuidanceLoop(opts: {
  getVideo: () => HTMLVideoElement | null;
  isActive: () => boolean;
  detect: (video: HTMLVideoElement, ts: number) => unknown;
  onGuidance: (g: FaceGuidance) => void;
  targetIntervalMs?: number; // start ~66–100
}): { start: () => void; stop: () => void; pause: () => void; resume: () => void }
```

- [ ] **Step 1: Failing test — loop never overlaps and adapts down**

Simulate slow `detect` (> interval): assert next tick skipped / interval increased; assert no concurrent detects.

- [ ] **Step 2: Implement landmarker init from `/mediapipe/wasm` + `/mediapipe/models/face_landmarker.task`**

`numFaces: 1`, `VIDEO` mode. On init failure → FaceScanner sets `guidanceDegraded=true` (manual-primary).

- [ ] **Step 3: Wire loop into FaceScanner**

Lifecycle per spec: init once when camera live; pause on tab hide / `active=false`; dispose on stop/unmount only.

When `guidance.stable && guidance.pose === requiredPose` && coordinator IDLE → auto `onCapture` with `source: "AUTO"`.

- [ ] **Step 4: Tests + manual smoke in browser (optional)**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(web): MediaPipe advisory guidance loop for FaceScanner"
```

---

### Task 5: Oval overlay + guidance copy

**Files:**
- Create: `web/components/face-scanner/overlay/OvalOverlay.tsx`
- Create: `web/components/face-scanner/overlay/GuidanceBanner.tsx`
- Create: `web/components/face-scanner/overlay/guidanceCopy.ts`
- Create: `web/components/face-scanner/overlay/guidanceCopy.test.ts`
- Modify: `FaceScanner.tsx`

**Interfaces:**
- Produces:

```ts
export function guidanceMessage(g: FaceGuidance, requiredPose: EnrollmentPose): string
// priority: no face → not centered → distance → pose → hold still → ready
```

- [ ] **Step 1: Failing tests for copy priority**

- [ ] **Step 2: Implement oval SVG/CSS mask mapped from source oval → display** (account for `object-fit: cover` + CSS mirror)

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(web): oval overlay and guidance copy for FaceScanner"
```

---

### Task 6: EnrollmentFlow cutover (UI-02, UI-06, UI-08)

**Files:**
- Create: `web/components/enrollment/EnrollmentFlow.tsx`
- Modify: `web/app/(admin)/employees/[id]/face/page.tsx`
- Keep: `web/features/enrollment/api.ts` unchanged

**Interfaces:**
- Consumes: `FaceScanner`, `captureEnrollmentPose`, `startEnrollment`, `commitEnrollment`, `abortEnrollment`
- Produces: page that stages FRONT/LEFT/RIGHT via `onCapture` → `CaptureResult`

```ts
async function handleCapture(candidate: CaptureCandidate): Promise<CaptureResult> {
  try {
    const res = await captureEnrollmentPose(employeeId, enrollmentId, requiredPose, candidate.blob);
    // Do NOT send candidate.guidance to API
    if (!res.accepted) return { status: "REJECTED" };
    // mark pose, advance requiredPose, keep scanner active
    return { status: "ACCEPTED" };
  } catch {
    return { status: "REJECTED" };
  }
}
```

- [ ] **Step 1: Implement EnrollmentFlow with progress 33/66/100 from accepted poses only**

- [ ] **Step 2: Wire face page; remove CameraCapture wizard buttons as primary UX**

- [ ] **Step 3: Manual check — enroll one employee through auto and one pose via Manual Capture**

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(web): EnrollmentFlow with guided FaceScanner"
```

---

### Task 7: Verify page `scannerActive` settle (UI-08)

**Files:**
- Modify: `web/app/(public)/verify/page.tsx`

**Interfaces:**
- After verify HTTP settles (matched true/false): `setScannerActive(false)` and stop/pause camera presentation
- No-match card: Try Again → `setScannerActive(true)`
- Hard/transport failure: `{ status: "REJECTED" }` keep active for retry
- `onCapture` must not attach `guidance` as request fields

- [ ] **Step 1: Replace CameraCapture with FaceScanner (`requiredPose="FRONT"`, `autoStart`)**

- [ ] **Step 2: Implement scannerActive transitions per spec §5 VerifyFlow**

- [ ] **Step 3: Manual check — stable face after no-match does not spam verify requests**

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(web): verify FaceScanner with scannerActive settle"
```

---

### Task 8: Remove legacy CameraCapture + DoD pass

**Files:**
- Delete: `web/components/camera/CameraCapture.tsx` (after no imports remain)
- Modify: `docs/superpowers/plans/2026-09-21-smartattend-mvp1.md` Task 7 note pointing to this plan (optional one-liner)
- Modify: README only if camera enrollment steps described

- [ ] **Step 1: Grep for CameraCapture imports; remove file**

- [ ] **Step 2: `cd web && npm test && npm run build`**

- [ ] **Step 3: Manual DoD checklist**

```text
[ ] mediapipe:verify passes
[ ] Enroll auto FRONT/LEFT/RIGHT + commit
[ ] Manual capture works when guidance imperfect
[ ] Verify match → attendance actions; camera not auto-looping
[ ] Verify no-match → Try Again required
[ ] Network: no continuous image uploads during guidance
[ ] Copy never says Face ID
```

- [ ] **Step 4: Commit**

```bash
git commit -m "chore(web): remove legacy CameraCapture after FaceScanner cutover"
```

---

## Spec coverage checklist

| Spec item | Task |
| --- | --- |
| SHA256 / exact pin / no @latest | 1 |
| Geometry / yaw LEFT sign / stability | 2 |
| Capture IDLE/CAPTURING/SUBMITTING/COOLDOWN | 3 |
| Adaptive loop / GPU→CPU / degrade | 4 |
| Oval + copy | 5 |
| EnrollmentFlow auto+manual | 6 |
| Verify scannerActive | 7 |
| Cleanup + DoD | 8 |
| Tasks 6/9 untouched | (none) |

## Plan self-review notes

- No TBD placeholders in steps
- Types consistent: `CaptureCandidate` / `CaptureResult` / `EnrollmentPose` / `FaceGuidance`
- Worker deferred; main-thread adaptive loop only
