# Canonical entry point for local development and CI. All test/lint/bench
# invocations should go through these targets so local and CI stay in sync.

COMPOSE_PROJECT ?= pgxkit-test
DOCKER_COMPOSE  := docker compose -p $(COMPOSE_PROJECT)

# Resolved lazily (recursive `=`) so it's only computed when a recipe needs it,
# after `test-db-up` has run and the container is listening.
TEST_DB_PORT      = $(shell $(DOCKER_COMPOSE) port postgres 5432 2>/dev/null | awk -F: '{print $$NF}')
TEST_DATABASE_URL = postgres://postgres:postgres@localhost:$(TEST_DB_PORT)/testdb?sslmode=disable

GO_TEST_FLAGS ?= -v -race -parallel=1

.PHONY: help test-db-up test-db-down test test-coverage coverage-html lint bench

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "Targets:\n"} /^[a-zA-Z_-]+:.*?##/ {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test-db-up: ## Start the test Postgres container (idempotent)
	$(DOCKER_COMPOSE) up -d --wait
	@echo "Postgres ready on host port $$($(DOCKER_COMPOSE) port postgres 5432 | awk -F: '{print $$NF}')"

test-db-down: ## Stop the test Postgres container and remove its volume
	$(DOCKER_COMPOSE) down -v

test: test-db-up ## Run the full test suite against the local test DB
	@TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test $(GO_TEST_FLAGS) ./...

test-coverage: test-db-up ## Run the test suite and write coverage.out
	@TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test $(GO_TEST_FLAGS) -coverprofile=coverage.out -covermode=atomic ./...

coverage-html: ## Open coverage.out in the browser (run after test-coverage)
	go tool cover -html=coverage.out

lint: ## Run golangci-lint
	golangci-lint run --timeout=5m

bench: test-db-up ## Run benchmarks against the local test DB
	@TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -bench=. -benchmem -run=^$$ ./...
