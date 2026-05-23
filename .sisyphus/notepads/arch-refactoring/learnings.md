# Learnings — arch-refactoring

## 2026-05-23 Session Start
- Plan: Monolith → AgentHarness refactoring
- 37 tasks across 6 waves + 4 final verification tasks
- Critical Path: T1 → T4 → T6 → T14 → T15 → T23 → T24 → T29 → T33 → T37 → F1-F4
- Wave 0 is fully parallelizable (T1, T2, T3)
- Wave 1 is mostly sequential (T4 must complete before T5, T5 before T6)

## 2026-05-23 Task 2: Security Fix
- server/config.yaml had real 360 Cloud API key on disk
- .gitignore had `**/config.yaml` (added in commit 2748f19 "fix ignore")
- config.yaml.template already had correct placeholder `YOUR_API_KEY_HERE`
- Git history analysis: real key `fk3542824333...` was NEVER committed — git history shows `REDACTED_API_KEY` in diffs
- The config.yaml file was gitignored before the real key was ever added, so the key only existed on disk
- Evidence file: .sisyphus/evidence/task-2-security-fix.txt

## 2026-05-23 Task 13: Ringbuf + GORM Conversion POC
- Ringbuf: `github.com/golang-cz/ringbuf@v0.4.0` — actively maintained, zero transitive deps
- GORM conversion test: 18 test cases all PASS, zero data loss confirmed
- Test file: `server/internal/storage/convert_test.go` — covers Session, Message, Todo round-trips
- Key finding: storage.Part.Interrupt is `any` while session.PersistedPart.Interrupt is `map[string]any` — `interruptToModel` handles the conversion properly