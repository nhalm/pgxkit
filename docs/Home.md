# Welcome to pgxkit Wiki

**pgxkit** is a lightweight, type-safe PostgreSQL toolkit for Go applications that provides a clean abstraction over pgx while maintaining performance and flexibility.

## Quick Navigation

### Documentation
- [Getting Started](Getting-Started) - Setup and basic usage
- [API Reference](API-Reference) - Complete API documentation
- [Examples](Examples) - Practical code examples and use cases

### Performance & Production
- [Performance Guide](Performance-Guide) - Optimization strategies and best practices
- [Production Guide](Production-Guide) - Deployment and production considerations
- [Testing Guide](Testing-Guide) - Testing strategies and golden tests

### Development
- [Contributing](Contributing) - How to contribute to pgxkit
- [FAQ](FAQ) - Frequently asked questions

## Key Features

- **Type-safe operations** with Go generics
- **Connection pooling** with read/write splitting
- **Golden testing** for reliable test suites
- **Extensible hooks** for monitoring and logging
- **Production-ready** with graceful shutdown
- **Zero dependencies** beyond pgx

## Quick Start

```go
package main

import (
    "context"
    "log"
    
    "github.com/nhalm/pgxkit"
)

func main() {
    // Connect to database
    db := pgxkit.NewDB()
    err := db.Connect(context.Background(), "postgres://user:pass@localhost/db")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()
    
    // Execute a query
    var name string
    err = db.QueryRow(context.Background(), 
        "SELECT name FROM users WHERE id = $1", 1).Scan(&name)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("User name: %s", name)
}
```

## Repository Information

- **GitHub**: [https://github.com/nhalm/pgxkit](https://github.com/nhalm/pgxkit)
- **Go Module**: `github.com/nhalm/pgxkit`
- **License**: MIT
- **Go Version**: 1.21+

## Community

- [Issues](https://github.com/nhalm/pgxkit/issues) - Report bugs or request features
- [Discussions](https://github.com/nhalm/pgxkit/discussions) - Community discussions
- [Pull Requests](https://github.com/nhalm/pgxkit/pulls) - Contribute code

## Recent Updates

This wiki is actively maintained and synchronized with the repository documentation. Check the [repository releases](https://github.com/nhalm/pgxkit/releases) for the latest updates.

---

*Last updated: December 2024* 