# Learnings

## 2026-05-26 Session Start
- Plan: ui-architecture-refactoring
- 22 implementation tasks + 3 final verification tasks
- 7 waves of execution
- Critical path: T2 → T5 → T6 → T10 → T20 → T21 → F1-F3
- Key constraint: T1 must complete before T2, T3, T4 (scaffold must exist first)
- chat-core: ZERO runtime deps, ZERO framework imports
- chat-react: < 200 lines total
- headless-hooks: ZERO framework imports
- Preserve filler-parts pattern and step 0 implicit behavior
