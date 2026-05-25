# SQLite Fallback - Learnings

## Key Findings

### SQLite vs PostgreSQL Model Adaptations
- `type:uuid` → `type:char(36)` for all UUID fields
- `type:jsonb` → `gorm:"serializer:json"` with `GormDataType()` returning `"text"`
- `type:uuid[]` → `type:text` with JSON serialization (Value: json.Marshal, Scan: json.Unmarshal)
- `default:gen_random_uuid()` → remove, use BeforeCreate hooks
- `constraint:OnDelete:CASCADE` → remove for SQLite (GORM handles relationships)

### SQLite-Specific Issues Found During Testing
1. **Test isolation**: `file::memory:?cache=shared` shares data between test DBs. Use `file:<testname>?mode=memory&cache=shared` instead.
2. **UpdateMetadata with maps**: SQLite cannot serialize `map[string]any` as a direct SQL parameter. Must wrap in `JSONB(metadata)` type for GORM to handle serialization.
3. **PRAGMA configuration**: Must set WAL, busy_timeout, foreign_keys, synchronous in DSN string for glebarez/sqlite driver.
4. **SetMaxOpenConns(1)**: Critical for SQLite single-writer safety.

### Code Quality Fixes Applied
1. SQL injection: `CREATE DATABASE " + dbName` → `fmt.Sprintf("CREATE DATABASE \"%s\"", dbName)` with quoted identifier
2. Ignored error: `sqlDB, _ := db.DB()` → proper error handling
3. Missing PRAGMA in init-db: Added same PRAGMA config as factory
4. `interface{}` → `any` in Scan methods (Go 1.26 style)
5. Build artifact: Added `server/init-db` to .gitignore

### Architecture Decisions
- SQLite provider is a complete independent package (no shared code with postgres)
- Factory pattern in `server/internal/store/` handles auto-detection
- Init-db has its own detection logic (separate binary, can't import factory without circular deps)
- Compile-time interface checks in test file (acceptable deviation from plan)

### Review Results
- F1 (Plan Compliance): APPROVE (6/6 Must Have, 7/7 Must NOT Have, 13/13 Deliverables)
- F2 (Code Quality): Fixed all HIGH/MEDIUM issues, APPROVE after fixes
- F3 (Manual QA): APPROVE (8/8 scenarios pass)
- F4 (Scope Fidelity): CONDITIONAL APPROVE (core/go.mod sqlite dep needed for tests)
