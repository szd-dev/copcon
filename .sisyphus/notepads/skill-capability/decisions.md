# Decisions: skill-capability

## 2026-06-06T06:14 Architecture Decisions

1. **Skill as plugin, not core** — User explicit decision
2. **Server-level explicit registration** — User chose this over core auto-integration
3. **Config + auto-discover** — User combined both for skill discovery paths
4. **TDD strategy** — User selected TDD (RED → GREEN → REFACTOR)
5. **CapabilityTypeModule** — Forced by harness architecture (CapabilityTypeSkill is dead code)
6. **Single tool with action param** — Avoids tool explosion (list/get/search)
7. **No per-agent filtering** — All agents see all skills in this iteration
8. **No allowed-tools enforcement** — Information-only, not enforced
9. **tests merge into impl tasks** — TDD means tests are written with each task
