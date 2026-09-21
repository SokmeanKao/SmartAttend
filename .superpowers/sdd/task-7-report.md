# Task 7 status
- Added reusable camera capture states: idle, permission, live, capturing, submitting, and error.
- Camera tracks stop on manual release and unmount, including permission-race cleanup.
- User-triggered captures resize to 1280px, encode as JPEG, and enforce the 5 MiB limit.
- Added FRONT → LEFT → RIGHT enrollment with accepted-pose progress and pose retakes.
- Complete enrollment commits through Task 6 APIs; navigation performs best-effort abort.
- Biometric embeddings and quality scores are never rendered.
- Added Enroll/Re-enroll face link to active employee details.
- Quality gate passed: ESLint, TypeScript, and production build.
