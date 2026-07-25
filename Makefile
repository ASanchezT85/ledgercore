# LedgerCore monorepo tasks.
# POSIX relative paths — run from the repo root. On Windows use Git Bash,
# or scripts/dev.ps1 for the compose targets.

COMPOSE := docker compose -f infra/compose/docker-compose.yml
GO_MODULES := libs/go services/ledger-core services/identity services/reconciliation services/webhooks

.PHONY: dev dev-obs down build-go test-go build-web fmt lint

## dev: start the local stack (postgres, nats, traefik and the four Go services)
dev:
	$(COMPOSE) up -d --build

## dev-obs: dev plus the observability profile (grafana/otel-lgtm on 3100)
dev-obs:
	$(COMPOSE) --profile obs up -d --build

## down: stop the stack (append "-v" by hand to also drop the data volume)
down:
	$(COMPOSE) down

## build-go: compile every Go module
build-go:
	@set -e; for dir in $(GO_MODULES); do \
		echo "==> go build $$dir"; \
		(cd $$dir && go build ./...); \
	done

## test-go: vet + test every Go module
test-go:
	@set -e; for dir in $(GO_MODULES); do \
		echo "==> go test $$dir"; \
		(cd $$dir && go vet ./... && go test ./...); \
	done

## build-web: install deps and build the console app
build-web:
	cd apps/console && pnpm install --frozen-lockfile=false && pnpm build

## fmt: gofmt -s -w over every Go module
fmt:
	@set -e; for dir in $(GO_MODULES); do \
		echo "==> gofmt $$dir"; \
		(cd $$dir && gofmt -s -w .); \
	done

## lint: go vet always; golangci-lint only if installed (optional, not required by CI)
lint:
	@set -e; for dir in $(GO_MODULES); do \
		echo "==> go vet $$dir"; \
		(cd $$dir && go vet ./...); \
	done
	@if command -v golangci-lint >/dev/null 2>&1; then \
		set -e; for dir in $(GO_MODULES); do \
			echo "==> golangci-lint $$dir"; \
			(cd $$dir && golangci-lint run ./...); \
		done; \
	else \
		echo "golangci-lint not installed — skipping (optional)"; \
	fi
