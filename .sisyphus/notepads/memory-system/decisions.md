# Decisions - Memory System

## Execution Order
- Wave 1 Batch A: T1 (Complete) + T2 (Types) — truly parallel
- Wave 1 Batch B: T3+T4 combined (facts + store extension) + T5 (register) + T6 (INDEX.md) — after Batch A
- Wave 1 merged: T3 and T4 modify same files (filememory.go, store_interface.go) — combine into single delegation
- T5 in Wave 1 only sets up struct/signatures, NOT hook expansion (deferred to T12)
