# pgxkit Product Requirements Document (PRD)

## Overview

**Product Name:** pgxkit  
**Version:** 2.0 (Breaking Change)  
**Target Users:** Go developers building PostgreSQL applications  
**Current Status:** Core DB implementation completed - refactoring from sqlc-specific toolkit to universal PostgreSQL toolkit  

## Implementation Status

### ✅ COMPLETED: Core DB Type with Read/Write Pool Abstraction (Issue #5)

**Implementation Date:** July 16, 2025  
**PR:** [#14](https://github.com/nhalm/pgxkit/pull/14)  
**Branch:** `implement-core-db-hooks`

**What was implemented:**
- **Core DB struct** with readPool, writePool, hooks, and shutdown fields
- **Constructor functions**: `NewDB(pool)` for single pool, `NewReadWriteDB(readPool, writePool)` for split pools
- **Default methods** (use write pool): `Query()`, `QueryRow()`, `Exec()`
- **Read optimization methods**: `ReadQuery()`, `ReadQueryRow()` for explicit read pool usage
- **Transaction support**: `BeginTx()` method using write pool
- **Statistics methods**: `Stats()`, `ReadStats()`, `WriteStats()`
- **Thread-safe shutdown**: `Shutdown(ctx)` with graceful pool closure
- **Dual-level hook system**: Operation-level hooks (HookFunc) + connection-level hooks
- **Hook types**: BeforeOperation, AfterOperation, BeforeTransaction, AfterTransaction, OnShutdown
- **Connection hooks**: OnConnect, OnDisconnect, OnAcquire, OnRelease
- **Comprehensive testing**: Unit tests covering all functionality including edge cases

**Key Technical Details:**
- `HookFunc` signature: `func(ctx context.Context, sql string, args []interface{}, operationErr error) error`
- Thread-safe implementation with `sync.RWMutex`
- Graceful handling of nil pools for testing
- Proper error propagation and hook execution order
- Import fix for `pgconn.CommandTag` vs `pgx.CommandTag`

**Files Created/Modified:**
- `db.go` - Main DB implementation with all core methods
- `hooks.go` - Extended with operation-level hooks and main Hooks struct
- `db_test.go` - Comprehensive unit tests for DB functionality
- `PRD.md` - Updated with implementation status

**All Requirements Met:**
- ✅ DB struct with readPool, writePool, hooks, and shutdown fields
- ✅ NewDB(pool) constructor for single pool usage
- ✅ NewReadWriteDB(readPool, writePool) constructor for split pools
- ✅ Query, QueryRow, Exec methods (default to write pool)
- ✅ ReadQuery, ReadQueryRow methods (explicit read pool usage)
- ✅ BeginTx method for transaction support
- ✅ Stats, ReadStats, WriteStats methods
- ✅ Thread-safe shutdown mechanism
- ✅ All methods properly route to correct pools
- ✅ Thread-safe implementation with proper mutex usage
- ✅ Comprehensive unit tests
- ✅ Documentation with examples

## Problem Statement

Go developers need production-ready PostgreSQL utilities that work with any code generation tool or raw pgx usage. The current pgxkit is tightly coupled to sqlc, limiting its usefulness as the ecosystem evolves (e.g., with new tools like Skimatik).

## Product Vision

pgxkit will be the go-to PostgreSQL production toolkit for Go applications, providing essential database utilities that work with any approach to PostgreSQL development.

## Core Principles

1. **Tool Agnostic** - Works with any code generation tool (sqlc, Skimatik, etc.) or raw pgx
2. **Safety First** - Defaults to safe operations (write pool) with explicit optimization paths
3. **Production Ready** - Built-in observability, error handling, and graceful shutdown
4. **Extensible** - Hook system for custom functionality
5. **Simple API** - Clean, intuitive interface following Go conventions

## Target Users

### Primary Users
- **Application Developers** - Building PostgreSQL-backed Go applications
- **DevOps Engineers** - Deploying and monitoring PostgreSQL applications
- **Library Authors** - Building tools that need PostgreSQL utilities

### User Personas
1. **Startup Developer** - Needs quick setup with production features
2. **Enterprise Developer** - Requires observability, testing, and reliability features
3. **Performance Engineer** - Needs read/write splitting and query optimization tools

## Core Features

### 1. ✅ Read/Write Pool Abstraction
**Priority:** P0 (Must Have) - **COMPLETED**

```go
// Single pool - same pool for read/write
db := pgxkit.NewDB(pool)

// Read/write split - separate pools
db := pgxkit.NewReadWriteDB(readPool, writePool)

// Default methods use write pool (safe)
db.Query(ctx, sql, args...)
db.Exec(ctx, sql, args...)

// Explicit read pool usage (optimization)
db.ReadQuery(ctx, sql, args...)
```

**Benefits:**
- Safe by default (write pool)
- Easy scaling path (add read replicas later)
- Transparent to application code

### 2. ✅ Query-Level Hooks
**Priority:** P0 (Must Have) - **COMPLETED**

**Implementation Date:** July 16, 2025  
**PR:** [#16](https://github.com/nhalm/pgxkit/pull/16)  
**Branch:** `implement-hook-pool-integration`

**What was implemented:**
- **Hook pool integration**: `ConfigurePool` methods to integrate connection hooks with pgxpool configuration
- **Dual-level hook execution**: Operation-level hooks execute during DB operations, connection-level hooks execute during pool lifecycle
- **Hook composition**: `CombineHooks` function to merge multiple hook sets
- **Pool configuration**: Proper integration with pgxpool's `AfterConnect` and `BeforeClose` callbacks
- **Comprehensive testing**: Tests verify hooks execute during actual pool operations
- **Documentation**: Extensive examples showing hook usage patterns

**Key Technical Details:**
- Connection hooks are integrated via `pgxpool.Config.AfterConnect` and `pgxpool.Config.BeforeClose`
- Preserves existing pool callbacks while adding hook functionality
- `Hooks.ConfigurePool(config)` method for easy integration
- `DB.Hooks()` method exposes hook manager for advanced configuration

```go
// Single hook API
db.AddHook("BeforeOperation", hookFunc)
db.AddHook("OnConnect", connectFunc)

// Operation-level hook types
BeforeOperation, AfterOperation, BeforeTransaction, AfterTransaction, OnShutdown

// Connection-level hook types (pgx lifecycle)
OnConnect, OnDisconnect, OnAcquire, OnRelease
```

**Hook Function Signature:**
```go
// Single hook function type - like middleware
type HookFunc func(ctx context.Context, sql string, args []interface{}, operationErr error) error
```

**Hook Execution Logic:**
- **BeforeOperation hooks**: `operationErr` is always `nil` (operation hasn't happened yet)
- **AfterOperation hooks**: `operationErr` contains the result of the operation (nil for success, error for failure)
- **Transaction hooks**: `sql` and `args` contain transaction info, `operationErr` shows transaction result
- **Shutdown hooks**: `sql` and `args` are empty strings/nil, `operationErr` is `nil`
- **Connection hooks**: Use pgx's native signatures for connection lifecycle management

**Benefits:**
- **Dual-level hooks**: Both operation-level (query/exec) and connection-level (pgx lifecycle)
- **Extensible architecture**: Circuit breakers, logging, tracing, metrics all as hooks
- **Middleware-like pattern**: Familiar to Go developers, single consistent API
- **Error handling**: Hook errors stop execution chain
- **pgx compatibility**: Full support for pgx's connection lifecycle hooks

### 3. Type Helpers
**Priority:** P1 (Should Have)

```go
// Clean architecture struct conversions
pgxkit.ToPgxText(stringPtr)
pgxkit.FromPgxText(pgxText)
pgxkit.ToPgxTimestamp(timePtr)
// ... etc for all pgx types
```

**Benefits:**
- Reduces boilerplate in repository layer
- Supports clean architecture patterns
- Consistent type handling across applications

### 4. Testing Infrastructure
**Priority:** P1 (Should Have)

```go
// Test database with setup/teardown
testDB := pgxkit.NewTestDB()
defer testDB.Close()

// Golden test support for query plans
testDB.EnableExplainGolden(t, "test_name")
repo.GetUsers(ctx) // Automatically captures EXPLAIN plans
```

**Benefits:**
- Catches performance regressions
- Supports multiple queries per test
- Automatic golden file management

### 5. Graceful Shutdown
**Priority:** P1 (Should Have) - **COMPLETED**

```go
// Graceful shutdown with timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
db.Shutdown(ctx)
```

**Benefits:**
- Production-ready deployment
- Prevents data corruption
- Configurable timeout handling

### 6. Production Features
**Priority:** P1 (Should Have)

- **Health Checks** - Database connectivity monitoring
- **Metrics Collection** - Query performance and connection stats
- **Error Handling** - Structured error types with context
- **Connection Lifecycle** - Proper resource management

## Technical Requirements

### Dependencies
- **Core:** `github.com/jackc/pgx/v5` and `github.com/jackc/pgx/v5/pgxpool`
- **Testing:** Standard library `testing` package
- **No Code Generation Dependencies** - Remove all sqlc imports

### Performance Requirements
- **Minimal Overhead** - Hooks should add <1ms latency
- **Connection Pooling** - Efficient pool management
- **Memory Usage** - Minimal memory footprint for hooks

### Compatibility
- **Go Version:** 1.21+
- **PostgreSQL:** 12+
- **Breaking Changes:** Accept breaking changes from v1.x

## API Design

### Core Types
```go
type DB struct {
    readPool   *pgxpool.Pool
    writePool  *pgxpool.Pool
    hooks      *Hooks
    // ... internal fields
}

type Hooks struct {
    // Operation-level hooks (query/exec operations)
    BeforeOperation   []HookFunc
    AfterOperation    []HookFunc
    BeforeTransaction []HookFunc
    AfterTransaction  []HookFunc
    OnShutdown        []HookFunc
    
    // Connection-level hooks (pgx lifecycle)
    OnConnect         []func(*pgx.Conn) error
    OnDisconnect      []func(*pgx.Conn)
    OnAcquire         []func(context.Context, *pgx.Conn) error
    OnRelease         []func(*pgx.Conn)
}
```

### Constructor Functions
```go
func NewDB(pool *pgxpool.Pool) *DB
func NewReadWriteDB(readPool, writePool *pgxpool.Pool) *DB
func NewTestDB() *TestDB
```

### Core Methods
```go
// Default methods (use write pool)
func (db *DB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
func (db *DB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
func (db *DB) Exec(ctx context.Context, sql string, args ...interface{}) (pgx.CommandTag, error)

// Explicit read methods (optimization)
func (db *DB) ReadQuery(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
func (db *DB) ReadQueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row

// Hook management
func (db *DB) AddHook(hookType string, hookFunc interface{}) *DB

// Lifecycle management
func (db *DB) Shutdown(ctx context.Context) error
func (db *DB) HealthCheck(ctx context.Context) error
```

## Success Metrics

### Adoption Metrics
- **GitHub Stars** - Community interest
- **Go Module Downloads** - Usage growth
- **Issue Resolution Time** - Developer experience

### Technical Metrics
- **Performance Overhead** - <1ms hook latency
- **Test Coverage** - >90% code coverage
- **Documentation Coverage** - All public APIs documented

### User Experience Metrics
- **Migration Ease** - Clear migration path from v1.x
- **Setup Time** - <5 minutes to working application
- **Feature Discovery** - Comprehensive examples and docs

## Migration Strategy

### Breaking Changes
- Remove all sqlc-specific interfaces
- New constructor functions
- Changed hook API

### Migration Path
1. **Documentation** - Comprehensive migration guide
2. **Examples** - Before/after code samples
3. **Tooling** - Migration scripts where possible
4. **Support** - Active issue resolution during migration period

## Timeline

### ✅ Phase 1: Core Implementation (Week 1-2) - COMPLETED
- ✅ New DB struct and constructors
- ✅ Hook system implementation
- ✅ Basic query methods
- ✅ Hook pool integration (Issue #6)

### Phase 2: Production Features (Week 3-4)
- Graceful shutdown
- Type helpers
- Error handling

### Phase 3: Testing & Polish (Week 5-6)
- TestDB implementation
- Golden test support
- Documentation and examples

### Phase 4: Release (Week 7)
- Migration guide
- Release notes
- Community announcement

## Risks & Mitigation

### Technical Risks
- **Performance Impact** - Mitigation: Benchmark all features
- **Complex Hook System** - Mitigation: Simple, well-tested implementation
- **Memory Leaks** - Mitigation: Proper resource management and testing

### Adoption Risks
- **Breaking Changes** - Mitigation: Clear migration path and documentation
- **Feature Gaps** - Mitigation: Comprehensive feature comparison with v1.x
- **Community Resistance** - Mitigation: Early feedback and transparent communication

## Success Criteria

### Must Have
- [x] Tool-agnostic design works with any PostgreSQL approach
- [x] Read/write pool abstraction with safe defaults
- [x] Extensible hook system for production features
- [x] Comprehensive test coverage and documentation

### Should Have
- [x] Performance overhead <1ms for hooks
- [ ] Golden test support for query plan regression testing
- [x] Graceful shutdown with configurable timeouts
- [ ] Migration guide with working examples

### Could Have
- [ ] Built-in metrics collection
- [ ] Advanced circuit breaker implementations
- [ ] Performance monitoring dashboard
- [ ] Integration with popular observability tools

## Open Questions

1. **Hook Type Validation** - Should we validate hook function signatures at runtime or compile time?
2. **Metrics Interface** - Should we define a metrics interface or use a specific library?
3. **Transaction Hooks** - How should transaction-level hooks interact with operation-level hooks?
4. **Golden Test Format** - JSON or text format for EXPLAIN plan storage?
5. **Backward Compatibility** - Should we provide any compatibility layer for v1.x users?
6. **Hook Performance** - Should we provide a way to disable hooks in production for maximum performance?

## Implementation Details

### Hook System Architecture
- **Dual-Level Design** - Operation-level hooks (HookFunc) and connection-level hooks (pgx signatures)
- **Hook Execution Order** - Hooks execute in the order they were added
- **Error Propagation** - First hook error stops execution chain
- **Thread Safety** - Hooks are protected by RWMutex for safe concurrent access
- **Hook Storage** - Each hook type stored as slice with appropriate function signature
- **Type Safety** - Runtime type checking for hook function signatures

### Golden Test Implementation
- **Multiple Queries** - Each query in a test function gets separate golden file
- **File Naming** - Format: `testdata/golden/TestName_query_1.json`
- **EXPLAIN Format** - JSON format with ANALYZE and BUFFERS options
- **Automatic Cleanup** - Test infrastructure handles golden file management

### Graceful Shutdown Process
1. **Stop Accepting New Operations** - Set shutdown flag
2. **Wait for Active Operations** - Use sync.WaitGroup
3. **Execute Shutdown Hooks** - Run OnShutdown hooks
4. **Close Pools** - Gracefully close read/write pools
5. **Timeout Handling** - Respect context timeout

## Core Files Structure

### 1. ✅ `hooks.go` - Hook System - COMPLETED
```go
// HookFunc is the universal hook function signature
type HookFunc func(ctx context.Context, sql string, args []interface{}, operationErr error) error

// Hooks manages both operation-level and connection-level hooks
type Hooks struct {
    mu sync.RWMutex
    
    // Operation-level hooks
    beforeOperation []HookFunc
    afterOperation  []HookFunc
    beforeTransaction []HookFunc
    afterTransaction []HookFunc
    onShutdown []HookFunc
    
    // Connection-level hooks (pgx native signatures)
    onConnect    []func(*pgx.Conn) error
    onDisconnect []func(*pgx.Conn)
    onAcquire    []func(context.Context, *pgx.Conn) error
    onRelease    []func(*pgx.Conn)
}

func NewHooks() *Hooks
func (h *Hooks) AddHook(hookType string, hookFunc HookFunc) error
func (h *Hooks) AddConnectionHook(hookType string, hookFunc interface{}) error
func (h *Hooks) ExecuteBeforeOperation(ctx context.Context, sql string, args []interface{}, operationErr error) error
func (h *Hooks) ExecuteAfterOperation(ctx context.Context, sql string, args []interface{}, operationErr error) error
func (h *Hooks) ExecuteBeforeTransaction(ctx context.Context, sql string, args []interface{}, operationErr error) error
func (h *Hooks) ExecuteAfterTransaction(ctx context.Context, sql string, args []interface{}, operationErr error) error
func (h *Hooks) ExecuteOnShutdown(ctx context.Context, sql string, args []interface{}, operationErr error) error
```

### 2. ✅ `db.go` - Main DB Type - COMPLETED
```go
// DB represents a database connection with read/write pool abstraction
type DB struct {
    readPool  *pgxpool.Pool
    writePool *pgxpool.Pool
    hooks     *Hooks
    mu        sync.RWMutex
    shutdown  bool
}

// Constructor functions
func NewDB(pool *pgxpool.Pool) *DB                                    // Single pool
func NewReadWriteDB(readPool, writePool *pgxpool.Pool) *DB           // Split pools

// Default methods (use write pool for safety)
func (db *DB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
func (db *DB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
func (db *DB) Exec(ctx context.Context, sql string, args ...interface{}) (pgx.CommandTag, error)

// Explicit read methods (use read pool for optimization)
func (db *DB) ReadQuery(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
func (db *DB) ReadQueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row

// Transaction support (always uses write pool)
func (db *DB) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)

// Hook management
func (db *DB) AddHook(hookType string, hookFunc HookFunc) error
func (db *DB) AddConnectionHook(hookType string, hookFunc interface{}) error

// Lifecycle management
func (db *DB) Shutdown(ctx context.Context) error
func (db *DB) Stats() *pgxpool.Stat
func (db *DB) ReadStats() *pgxpool.Stat
func (db *DB) WriteStats() *pgxpool.Stat
```

### 3. `types.go` - Type Helpers
```go
// Clean up existing pgx_helpers.go into organized type conversion utilities
// Keep existing functions like ToPgxText, FromPgxText, etc.
// Add any missing pgx type conversions

func ToPgxText(s *string) pgtype.Text
func FromPgxText(t pgtype.Text) *string
func ToPgxInt8(i *int64) pgtype.Int8
func FromPgxInt8(i pgtype.Int8) *int64
func ToPgxUUID(id uuid.UUID) pgtype.UUID
func FromPgxUUID(pgxID pgtype.UUID) uuid.UUID
func ToPgxTimestamptz(t *time.Time) pgtype.Timestamptz
func FromPgxTimestamptz(ts pgtype.Timestamptz) time.Time
// ... additional type helpers as needed
```

### 4. `test_db.go` - Testing Infrastructure
```go
type TestDB struct {
    *DB
    goldenEnabled bool
    testName      string
    t             *testing.T
}

func NewTestDB() *TestDB
func (tdb *TestDB) EnableExplainGolden(t *testing.T, testName string)
func (tdb *TestDB) captureExplainPlan(ctx context.Context, sql string, args []interface{})
func RequireTestDB(t *testing.T) *TestDB  // Skips if no test DB available

// Golden test capture logic for EXPLAIN plans
// - Automatically captures EXPLAIN (ANALYZE, BUFFERS) plans
// - Creates separate golden files for each query
// - File naming: testdata/golden/TestName_query_1.json
```

### 5. Clean Up Tasks
**Files to Remove:**
- `connection.go` (functionality moved to `db.go`)
- `readwrite.go` (functionality moved to `db.go`)
- Any sqlc-specific imports or dependencies

**Files to Keep and Update:**
- `retry.go` (still useful for production)
- `logging.go` (still useful for production)
- `errors.go` (still useful for structured error handling)
- `pgx_helpers.go` → rename to `types.go` and clean up

**Files to Update:**
- `README.md` - Update for v2.0 tool-agnostic approach
- `go.mod` - Remove any sqlc dependencies
- `examples.md` - Update examples for new API

## Key Design Points

1. **Safety First**: All default methods (`Query`, `Exec`, `QueryRow`) use write pool
2. **Explicit Optimization**: `ReadQuery`, `ReadQueryRow` use read pool
3. **Single Hook API**: `db.AddHook(hookType, hookFunc)` for all operation-level hooks
4. **Dual Hook System**: Operation-level hooks (HookFunc) + connection-level hooks (pgx native)
5. **Tool Agnostic**: No dependencies on sqlc or any specific code generation tool
6. **Production Ready**: Graceful shutdown, statistics, error handling
7. **Testing First**: Golden test support for performance regression detection

## Documentation Requirements

### API Documentation
- [ ] Comprehensive godoc for all public APIs
- [ ] Code examples for each major feature
- [ ] Migration guide from v1.x to v2.0

### User Guides
- [ ] Quick start guide (5-minute setup)
- [ ] Production deployment guide
- [ ] Testing best practices
- [ ] Performance optimization guide

### Developer Documentation
- [ ] Contributing guidelines
- [ ] Architecture overview
- [ ] Testing strategy
- [ ] Release process 