# Contributing to pgxkit

**[← Back to Home](Home)**

Thank you for your interest in contributing to pgxkit! This guide will help you get started with contributing to the project.

## Table of Contents

1. [Getting Started](#getting-started)
2. [Development Setup](#development-setup)
3. [Code Standards](#code-standards)
4. [Testing Guidelines](#testing-guidelines)
5. [Pull Request Process](#pull-request-process)
6. [Issue Guidelines](#issue-guidelines)
7. [Code Review Process](#code-review-process)
8. [Release Process](#release-process)

## Getting Started

### Prerequisites

- Go 1.25 or later
- Docker (for the local test database)
- Git
- Make

### Repository Structure

```
pgxkit/
├── db.go              # Main DB implementation
├── hooks.go           # Hook system
├── types.go           # Type conversion helpers
├── retry.go           # Retry logic
├── errors.go          # Error handling
├── test_db.go         # Testing utilities
├── *_test.go          # Test files
├── wiki/              # Wiki documentation
├── examples.md        # Usage examples
└── README.md          # Project overview
```

## Development Setup

### 1. Fork and Clone

```bash
# Fork the repository on GitHub, then:
git clone https://github.com/yourusername/pgxkit.git
cd pgxkit
```

### 2. Set Up Development Environment

```bash
go mod download
```

### 3. Run Tests

The `Makefile` is the canonical entry point — it spins up a containerized
Postgres, exports the right `TEST_DATABASE_URL`, and runs the suite the same
way CI does. Always use it instead of invoking `go test` directly.

```bash
make test-db-up    # start Postgres (Docker assigns a free host port)
make test          # run the full suite
make test-db-down  # tear down when done
```

Other useful targets:

```bash
make test-coverage   # writes coverage.out
make coverage-html   # open the HTML coverage report
make bench           # run benchmarks
make lint            # run golangci-lint
make help            # list all targets
```

If you want to point the suite at an existing Postgres instance instead of the
Docker one, set `TEST_DATABASE_URL` directly and run `go test ./...`.

### 4. Run Linting

```bash
make lint
```

Install `golangci-lint` first if you don't have it:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

## Code Standards

### Go Code Style

We follow standard Go conventions with some additional guidelines:

#### Formatting
- Use `gofmt` for code formatting
- Use `goimports` for import organization
- Maximum line length: 120 characters
- Use tabs for indentation

#### Naming Conventions
- Use descriptive names for functions and variables
- Follow Go naming conventions (CamelCase for exported, camelCase for internal)
- Use meaningful package names

#### Comments
- All exported functions, types, and constants must have comments
- Comments should explain "why", not just "what"
- Use complete sentences in comments
- Start comments with the name of the item being documented

**Example:**
```go
// Connect establishes a database connection with a single pool.
// If dsn is empty, it uses environment variables to construct the connection string.
// The hooks are configured at pool creation time for proper integration.
func (db *DB) Connect(ctx context.Context, dsn string) error {
    // Implementation details...
}
```

#### Error Handling
- Always handle errors explicitly
- Use wrapped errors with context: `fmt.Errorf("operation failed: %w", err)`
- Don't ignore errors silently
- Provide meaningful error messages

**Example:**
```go
pool, err := pgxpool.NewWithConfig(ctx, config)
if err != nil {
    return fmt.Errorf("failed to create connection pool: %w", err)
}
```

#### Function Design
- Keep functions small and focused
- Use interfaces for better testability
- Prefer composition over inheritance
- Use context.Context for cancellation

### Documentation Standards

#### Code Documentation
- Document all public APIs
- Include usage examples in comments
- Document non-obvious behavior
- Use godoc format

#### Wiki Documentation
- Write clear, concise explanations
- Include practical examples
- Add cross-references between pages
- Keep content up to date with code changes

## Testing Guidelines

### Test Organization

#### Test File Structure
- Name test files `*_test.go`
- Group related tests in the same file
- Use descriptive test function names

**Example:**
```go
func TestDB_Connect_WithValidDSN_ReturnsNoError(t *testing.T) {
    // Test implementation
}

func TestDB_Connect_WithInvalidDSN_ReturnsError(t *testing.T) {
    // Test implementation
}
```

#### Test Categories

1. **Unit Tests** - Test individual functions in isolation
2. **Integration Tests** - Test interactions with real database
3. **Plan-Regression Tests** - Detect query plan shape changes
4. **Benchmark Tests** - Measure performance characteristics

### Writing Good Tests

#### Test Structure
Use the AAA pattern: Arrange, Act, Assert

```go
func TestUserRepository_CreateUser(t *testing.T) {
    // Arrange
    suite := NewTestSuite(t)
    repo := NewUserRepository(suite.DB)
    user := &User{Name: "John", Email: "john@example.com"}
    
    // Act
    err := repo.CreateUser(suite.ctx, user)
    
    // Assert
    require.NoError(t, err)
    assert.NotZero(t, user.ID)
}
```

#### Test Data Management
- Use fixtures for complex test data
- Create test data in tests, don't rely on external state
- Clean up test data appropriately
- Use factory patterns for test object creation

#### Assertions
- Use `require` for critical assertions that should stop the test
- Use `assert` for additional checks
- Provide meaningful assertion messages

```go
require.NoError(t, err, "Failed to create user")
assert.Equal(t, expected.Name, actual.Name, "User name should match")
```

### Performance Tests

#### Benchmarks
Write benchmarks for performance-critical code:

```go
func BenchmarkDB_Query(b *testing.B) {
    db := setupBenchmarkDB(b)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        rows, err := db.Query(ctx, "SELECT id FROM users LIMIT 10")
        if err != nil {
            b.Fatal(err)
        }
        rows.Close()
    }
}
```

#### Plan-Regression Tests
Use plan-regression tests to catch query plan shape changes (e.g. seq-scan to index-scan, nested-loop to hash-join, a new sort node, a different join order). The captured plan comes from `EXPLAIN (FORMAT JSON, COSTS OFF)`; result rows are not compared.

```go
func TestComplexQuery_Plan(t *testing.T) {
    testDB := setupTestDB(t)
    db := testDB.EnableAssertPlan("TestComplexQuery")

    // Structural query plan will be captured and asserted
    rows, err := db.Query(ctx, complexQuery)
    require.NoError(t, err)
    defer rows.Close()
}
```

## Pull Request Process

### Before Submitting

1. **Run all tests locally**
   ```bash
   make test
   ```

2. **Run linting**
   ```bash
   make lint
   ```

3. **Update documentation** if needed

4. **Add/update tests** for new functionality

### PR Guidelines

#### Title and Description
- Use clear, descriptive titles
- Reference related issues: "Fixes #123" or "Closes #456"
- Explain what the change does and why
- Include any breaking changes

#### Size and Scope
- Keep PRs focused and reasonably sized
- One logical change per PR
- Split large changes into multiple PRs

#### Checklist
- [ ] Tests pass locally
- [ ] Linting passes
- [ ] Documentation updated
- [ ] Tests added/updated
- [ ] No breaking changes (or clearly documented)

### Example PR Description

```markdown
## Summary
Add retry logic with exponential backoff for connection failures.

## Test plan
- Unit tests for retry logic
- Integration tests with connection failures
- Verified exponential backoff behavior

Fixes #123
```

## Issue Guidelines

### Reporting Bugs

#### Bug Report Template
```markdown
## Bug Description
A clear description of what the bug is.

## To Reproduce
Steps to reproduce the behavior:
1. Go to '...'
2. Click on '....'
3. Scroll down to '....'
4. See error

## Expected Behavior
What you expected to happen.

## Environment
- Go version: [e.g. 1.21]
- pgxkit version: [e.g. v1.0.0]
- PostgreSQL version: [e.g. 14.5]
- OS: [e.g. macOS 12.0]

## Additional Context
Any other context about the problem.
```

### Feature Requests

#### Feature Request Template
```markdown
## Feature Description
A clear description of what you want to happen.

## Use Case
Explain the problem this feature would solve.

## Proposed Solution
Describe the solution you'd like.

## Alternatives Considered
Any alternative solutions or features you've considered.

## Additional Context
Any other context or screenshots about the feature request.
```

### Issue Labels

- `bug` - Something isn't working
- `enhancement` - New feature or request
- `documentation` - Improvements or additions to documentation
- `good first issue` - Good for newcomers
- `help wanted` - Extra attention is needed
- `question` - Further information is requested

## Code Review Process

### For Contributors

#### Responding to Feedback
- Address all feedback promptly
- Ask questions if feedback is unclear
- Make requested changes in new commits
- Don't force-push after review has started

#### Code Review Etiquette
- Be open to feedback
- Explain your reasoning for decisions
- Keep discussions focused and professional
- Thank reviewers for their time

### For Reviewers

#### Review Checklist
- [ ] Code follows style guidelines
- [ ] Tests are adequate and pass
- [ ] Documentation is updated
- [ ] No obvious bugs or issues
- [ ] Performance implications considered
- [ ] Security implications considered

#### Review Comments
- Be constructive and specific
- Explain the "why" behind suggestions
- Distinguish between blocking and non-blocking feedback
- Acknowledge good practices

## Release Process

### Versioning

We follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Incompatible API changes
- **MINOR**: New functionality (backward compatible)
- **PATCH**: Bug fixes (backward compatible)

### Release Checklist

1. Update version in relevant files
2. Run all tests
3. Create release tag
4. Update documentation

## Getting Help

### Communication Channels

- **GitHub Issues** - Bug reports and feature requests
- **GitHub Discussions** - Questions and general discussion
- **Code Reviews** - PR feedback and technical discussion

### Development Questions

If you have questions about:
- **Code architecture** - Open a discussion
- **Implementation details** - Comment on relevant PR/issue
- **Testing strategy** - Ask in discussions
- **Performance** - Open an issue with benchmarks

## Recognition

Contributors are recognized in:
- Release notes
- README acknowledgments
- GitHub contributors list

Thank you for contributing to pgxkit! Your efforts help make database development in Go better for everyone.

## See Also

- **[Getting Started](Getting-Started)** - Basic setup and usage
- **[Testing Guide](Testing-Guide)** - Detailed testing information
- **[API Reference](API-Reference)** - Complete API documentation
- **[Examples](Examples)** - Usage examples

---

**[← Back to Home](Home)**

*This contributing guide helps ensure high-quality contributions to pgxkit. Thank you for helping improve the project!* 