# Guided Face Scanner (Hybrid) — Design

**Date:** 2026-09-22  
**Status:** Ready for review  
**Amends:** `2026-09-21-smartattend-mvp1-design.md` (UI-02, UI-05, Enrollment/Verify UX, Camera capture; adds UI-08, FACE-GUIDE-02…05)  
**Does not change:** Enrollment/verify Go APIs, `face_templates` schema, YuNet/SFace authority (Tasks 6, 8, 9, 10)

## 1. Goal and constraints

Upgrade enrollment and verification **interaction** to an oval, Face-ID-*style* guided experience with automatic capture and a manual fallback. Backend still stores exactly three poses: `FRONT`, `LEFT`, `RIGHT`.

**Marketing:** “Guided face enrollment / verification.” Never claim Face ID technology or Face-ID-level security (no TrueDepth / IR / Secure Enclave).

**Hybrid UX**

```text
Primary: oval scanner → auto FRONT/LEFT/RIGHT (enroll) or one front-like capture (verify)
Fallback: Manual Capture → same APIs
Both → same templates / same verify / same commit
```

**Hybrid detectors**

```text
Browser: MediaPipe Face Landmarker (advisory guidance only)
Server:  YuNet (accept pose/quality) → SFace (embed / match)
```

## 2. Architecture (frozen)

```text
EnrollmentFlow / VerifyFlow     mode business (poses, APIs, receipts, scannerActive)
        ↓
FaceScanner                     camera, oval, MediaPipe, stability, capture guard
        ↓
CaptureCandidate (JPEG only)
        ↓
Existing Go enroll / verify APIs
        ↓
YuNet / SFace (authoritative)
```

| `FaceScanner` owns | Flow owns |
| --- | --- |
| `getUserMedia` + device selection UI | enroll start / abort / commit |
| MediaPipe init / pause / dispose | `requiredPose` FRONT→LEFT→RIGHT |
| Adaptive guidance loop | API capture / verify semantics |
| Oval, distance, pose-band, stability | Server error → UX copy |
| Auto + manual capture trigger | Accepted pose progression |
| In-flight capture guard | Verify receipt + attendance |
| JPEG normalization (≤1280, ≤5 MiB) | `scannerActive` / pause after settle |
| Emits `onCapture` only | Navigation / complete |

**Capture contract**

```ts
type CaptureResult =
  | { status: "ACCEPTED" }
  | { status: "REJECTED" };

type FaceScannerProps = {
  requiredPose: EnrollmentPose; // verify always passes FRONT for guidance
  active?: boolean;             // false → pause loop / no auto; flow-controlled
  onCapture: (candidate: CaptureCandidate) => Promise<CaptureResult>;
};
```

`FaceScanner` does not interpret enrollment or match outcomes beyond unlocking after `onCapture` settles when `active` remains true.

## 3. Guidance model and state machine (frozen)

```ts
type GuidancePose = "LEFT" | "FRONT" | "RIGHT" | "UNKNOWN";
type EnrollmentPose = "LEFT" | "FRONT" | "RIGHT";

type FaceGuidance = {
  faceDetected: boolean;
  centered: boolean;
  distance: "TOO_CLOSE" | "GOOD" | "TOO_FAR";
  pose: GuidancePose;
  stable: boolean;
};

type CaptureCandidate = {
  blob: Blob;
  source: "AUTO" | "MANUAL";
  guidance: FaceGuidance; // diagnostics / UI only; never trusted server pose metadata
};
```

`guidance.pose === requiredPose` never mutates enrollment progress. Only HTTP success from the enroll capture endpoint advances FRONT→LEFT→RIGHT.

**Guidance copy priority (first failure wins)**

1. No face → position in frame  
2. Not centered → center in oval  
3. Distance → move closer / move back  
4. Pose mismatch → look forward / turn slightly left / right  
5. Not stable → hold still  
6. Else → hold still (auto about to fire) / ready  

**Scanner capture states**

```text
IDLE → (auto-ready OR manual) → CAPTURING → SUBMITTING
  → ACCEPTED | REJECTED → COOLDOWN (~300–500 ms) → IDLE
```

While state ≠ `IDLE`: auto disabled, manual disabled, stability accumulation reset/disabled.

**Async settlement**

```text
CAPTURING → create JPEG → SUBMITTING → await onCapture(candidate)
  ACCEPTED → reset stability → COOLDOWN → IDLE
  REJECTED → reset stability → COOLDOWN → IDLE
```

- **EnrollmentFlow:** `ACCEPTED` only when capture HTTP 200 and pose accepted; then advance `requiredPose`; **scanner stays active**.  
- **VerifyFlow:** `REJECTED` on transport/hard API failure (stay on scanner if still active). On settled verify response (matched or not): return `ACCEPTED` to unlock scanner **and set `scannerActive=false`** so auto-capture cannot fire again until Try Again.

**Manual fallback**

Enabled when `active` ∧ camera live ∧ scanner `IDLE`. May bypass centered / distance / pose / stable. Must not bypass live camera, IDLE guard, or valid frame. Server remains authoritative.

**MediaPipe lifecycle**

```text
mounted + camera live + active → init landmarker once → start loop
tab hidden OR active=false → pause loop, reset stability; keep landmarker
tab visible AND active → resume loop
camera stop / unmount → stop loop, stop tracks, dispose landmarker
```

## 4. MediaPipe assets, runtime & heuristics (frozen)

### 4.1 Asset pinning by bytes

| Asset | Rule |
| --- | --- |
| npm | `@mediapipe/tasks-vision` **exact** semver in `package.json` / lockfile (`1.0.1` candidate; no `^` / `~`) |
| WASM | Copied from **that exact package version** into `web/public/mediapipe/wasm/` |
| Model | Vendored same-origin `web/public/mediapipe/models/face_landmarker.task` (Face Landmarker float16 bundle) |
| Checksum | `face_landmarker.task` **SHA256 pinned** in repo (manifest or test fixture) |
| CI/test | Assertion fails if model bytes hash ≠ pin |
| URLs | No `@latest` CDN loads for WASM or model |

### 4.2 Adaptive guidance cadence

`detectForVideo()` is synchronous on the main thread. Worker deferred.

```text
Target cadence: 10–15 FPS (not a DoD requirement on every machine)
Minimum interval: ~67–100 ms
Never overlap inference calls
Skip duplicate / unchanged video frames
Adapt downward if inference is slow:
  fast ≈ 15 FPS → normal ≈ 10 → weak ≈ 5
Persistent jank / both delegates fail → manual-primary
```

GPU preferred; on GPU init failure retry CPU; if both fail → manual-primary with non-blocking notice.

### 4.3 Coordinate and mirroring convention

```text
All guidance math: intrinsic video/sensor coordinates
  (video.videoWidth × video.videoHeight), UNMIRRORED
Preview: may be CSS-mirrored for familiarity
Overlay: mapped from source → rendered preview separately
```

**Yaw convention (locked):**

```text
negative yaw proxy = user's LEFT
positive yaw proxy = user's RIGHT
```

Tests must cover: mirrored preview does not invert guidance; landscape/portrait; `object-fit: cover`; mismatched video/container aspect ratios.

### 4.4 Oval, distance, pose bands

**Oval:** center ≈ frame center; width ≈ `0.55 × min(W,H)`; height ≈ `width × 1.25`; `centered` = face center inside ~85% radii.

**Distance** (`faceHeight / ovalHeight`): `< 0.55` TOO_FAR; `> 0.95` TOO_CLOSE; else GOOD. Client config only.

**Pose bands** (nose vs eye midline / inter-ocular, with hysteresis): `|yaw| ≤ frontMax` → FRONT; `yaw ≤ −sideMin` → LEFT; `yaw ≥ +sideMin` → RIGHT; else UNKNOWN. Starting `frontMax` / `sideMin` / ε chosen in the plan and tuned empirically; **must not** claim equality with YuNet thresholds.

**Stability (FACE-GUIDE-04):** `stableSince` timestamp; default window **400 ms**; reset on readiness fail, `requiredPose` change, capture start, loop pause / `active=false`.

**`numFaces: 1`:** MediaPipe config for smoothing only. Client does **not** claim exactly one person is present; YuNet remains responsible for `MULTIPLE_FACES`.

## 5. Enrollment and verify UX (frozen)

**Shared shell:** oval mask, guidance text, Manual Capture (when IDLE), no Face ID marketing claims.

**EnrollmentFlow**

```text
start → FaceScanner(active, requiredPose=FRONT)
capture ACCEPTED → mark ✓, next pose, progress 33/66/100, scanner stays active
all three → Complete → commit
```

Retake sets `requiredPose`; next accept replaces staged template. Abort/unmount best-effort; TTL authoritative.

**VerifyFlow**

```text
employee code → FaceScanner(active=true, requiredPose=FRONT, auto-start)
capture → verifying
matched=true  → scannerActive=false → stop/pause camera → receipt + Check In/Out
matched=false → scannerActive=false → "Identity not verified" + [Try Again]
                 Try Again → scannerActive=true
```

No LEFT/RIGHT loop. No-match is **settled** — not an invitation to auto-submit again while the face remains stable.

**Device selection:** Device picker available; default `facingMode: "user"` where supported. **No security decision** based on whether a browser camera appears physical or virtual. Virtual-camera resistance is future anti-spoof/liveness work.

## 6. Requirements

| ID | Requirement |
| --- | --- |
| **UI-02** | Enrollment uses an oval face-scanner that automatically captures accepted FRONT, LEFT, and RIGHT poses as the user follows directional guidance. Manual Capture remains available when automatic guidance cannot complete reliably. Both paths produce exactly three staged templates and use the existing enrollment commit flow. |
| **UI-05** | Only explicit AUTO or MANUAL `CaptureCandidate` frames are uploaded. Continuous guidance processing stays in the browser. |
| **UI-08** | Shared `FaceScanner` for enrollment and verification. Enrollment collects FRONT/LEFT/RIGHT; verification captures one stable front-like face. Mode-specific business workflow (including `scannerActive`) remains outside the scanner. |
| **FACE-GUIDE-02** | MediaPipe guidance is advisory and may only determine when SmartAttend should attempt a capture. Client-side guidance never marks enrollment or verification successful. Acceptance remains exclusively determined by the server-side YuNet/SFace pipeline. |
| **FACE-GUIDE-03** | Continuous guidance processing remains local to the browser. Guidance frames are never continuously uploaded; only explicit auto-capture or manual-capture candidates are sent to the Go API. |
| **FACE-GUIDE-04** | Stability is earned only after all advisory capture-readiness conditions remain continuously satisfied for the configured stability window. Any readiness failure, required-pose change, capture attempt, or guidance-loop suspension resets stability. |
| **FACE-GUIDE-05** | Client MediaPipe assets (npm package, WASM, `.task` model) are exact-version / checksum-pinned and served same-origin. Guidance cadence is adaptive; 10–15 FPS is a target, not a universal acceptance gate. |

## 7. Testing and DoD

**Asset/runtime:** exact npm version; no `@latest` URLs; model SHA256 verification; GPU fail → CPU retry; both fail → manual-primary.

**Geometry:** mirrored preview does not invert guidance; source/display aspect mapping; yaw hysteresis; stability resets on `requiredPose` change.

**Capture:** simultaneous manual+auto → one candidate; SUBMITTING never emits another; verify no-match does not auto-retry; server reject keeps same enrollment pose.

**Privacy:** guidance loop generates zero HTTP image requests; `CaptureCandidate.guidance` is not sent as trusted pose metadata.

**Manual DoD:** enroll auto + manual fallback; verify oval one-shot with Try Again; network panel shows no continuous image upload.

## 8. Non-goals

- Face ID / TrueDepth / liveness / virtual-camera security claims  
- Continuous upload or new face-service guide API  
- Tasks 6/9 / DB / embedding contract changes  
- MediaPipe Web Worker (deferred unless main-thread jank forces revisit)  
- 1:N identification  

## 9. Implementation touchpoints

```text
web/components/face-scanner/
  FaceScanner.tsx
  camera/
  guidance/          # MediaPipe + heuristics → FaceGuidance
  overlay/
web/components/enrollment/EnrollmentFlow.tsx
web/app/(public)/verify/  # VerifyFlow composition + scannerActive
web/public/mediapipe/{wasm,models}/
web/…/mediapipe.manifest.json  # or equivalent SHA256 pin
```

Primary plan impact: **Task 7** (and shared scanner wiring on verify UI). Tasks 6 and 9 unchanged.
