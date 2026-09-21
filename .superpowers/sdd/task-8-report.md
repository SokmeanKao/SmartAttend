# Task 8 Report
Status: COMPLETE — FACE-BIO-01…06 biometric core implemented.
- Pinned YuNet 2023mar and SFace 2021dec downloads verify immutable SHA256 values.
- Startup re-verifies both artifacts and refuses model absence or checksum mismatch.
- Embed now performs bounded JPEG/PNG EXIF decode to BGR, YuNet selection, face-region quality, landmark pose validation, alignCrop, SFace, L2 normalization, and float32-le base64 encoding.
- Pose and quality bands are environment-configurable; out-of-band captures return `INVALID_POSE`.
- Images and embeddings are not logged; the model threshold default remains `0.363`.
- Tests: `pytest face-service/tests` → 19 passed.
- Image gate: `docker compose build face-service` succeeded; image tests → 19 passed.
Blockers: none.
