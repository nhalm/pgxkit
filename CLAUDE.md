# CLAUDE.md

Repo-specific guidance for Claude Code sessions working in pgxkit.

## Always use the Makefile

The `Makefile` is the canonical entry point for tests, lint, and benchmarks —
locally and in CI. Do **not** invoke `go test`, `golangci-lint`, or
`docker compose` directly.

| Task                | Use                  |
| ------------------- | -------------------- |
| Run the test suite  | `make test`          |
| Run with coverage   | `make test-coverage` |
| Run benchmarks      | `make bench`         |
| Run the linter      | `make lint`          |
| Start the test DB   | `make test-db-up`    |
| Stop the test DB    | `make test-db-down`  |
| List all targets    | `make help`          |

The DB targets use `docker-compose.yml` and let Docker assign a free host port
(no fixed 5432) to avoid clashes with any local Postgres. `make test` and
friends discover that port at runtime and inject `TEST_DATABASE_URL`.

If a target you need doesn't exist, add it to the `Makefile` rather than
running the underlying command ad-hoc — that's how local and CI drift apart.
