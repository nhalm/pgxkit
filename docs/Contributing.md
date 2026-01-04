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

- Go 1.21 or later
- PostgreSQL 12 or later (for testing)
- Git
- Make (optional, for convenience commands)

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
# Install dependencies
go mod download

# Set up test database
export TEST_POSTGRES_HOST=localhost
export TEST_POSTGRES_PORT=5432
export TEST_POSTGRES_USER=postgres
export TEST_POSTGRES_PASSWORD=yourpassword
export TEST_POSTGRES_DB=pgxkit_test
export TEST_POSTGRES_SSLMODE=disable
```

### 3. Run Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific test
go test -run TestSpecificFunction ./...
```

### 4. Run Linting

```bash
# Install golangci-lint if not already installed
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linting
golangci-lint run
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
3. **Golden Tests** - Test performance regression detection
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

#### Golden Tests
Use golden tests for query plan regression detection:

```go
func TestComplexQuery_Golden(t *testing.T) {
    testDB := setupTestDB(t)
    db := testDB.EnableGolden("TestComplexQuery")
    
    // Query plan will be captured and compared
    rows, err := db.Query(ctx, complexQuery)
    require.NoError(t, err)
    defer rows.Close()
}
```

## Pull Request Process

### Before Submitting

1. **Run all tests locally**
   ```bash
   go test ./...
   ```

2. **Run linting**
   ```bash
   golangci-lint run
   ```

3. **Update documentation** if needed

4. **Add/update tests** for new functionality

5. **Update CHANGELOG** if applicable

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
- [ ] CHANGELOG updated (if applicable)
- [ ] No breaking changes (or clearly documented)

### Example PR Description

```markdown
## Summary
Add retry logic with exponential backoff for connection failures.

## Changes
- Add `RetryConfig` type for configuration
- Implement `ExecWithRetry` method
- Add tests for retry scenarios
- Update documentation with retry examples

## Testing
- Added unit tests for retry logic
- Added integration tests with connection failures
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
2. Update CHANGELOG.md
3. Run all tests
4. Create release tag
5. Update documentation
6. Announce release

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