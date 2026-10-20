GOLANGCI_LINT_VERSION := v2.12.2
GOLANGCI_LINT ?= $(CURDIR)/.bin/golangci-lint
GOLANGCI_LINT_CACHE := $(CURDIR)/.cache/golangci-lint
export GOLANGCI_LINT_CACHE

VERSION ?= dev
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= unknown
BUILD_LDFLAGS := -X campfire/internal/shared/buildinfo.Version=$(VERSION) -X campfire/internal/shared/buildinfo.Commit=$(COMMIT) -X campfire/internal/shared/buildinfo.Date=$(BUILD_DATE)

.PHONY: dependencies dependencies-backend dependencies-frontend \
	build build-ui build-campfire build-spark test test-backend test-frontend test-postgres \
	format format-backend format-frontend format-check format-check-backend format-check-frontend \
	lint lint-backend lint-frontend vet check tools dev dev-mock dev-ui preview

dependencies: dependencies-backend dependencies-frontend

dependencies-backend:
	go -C backend mod download

dependencies-frontend:
	pnpm --dir web install --frozen-lockfile

build: build-campfire build-spark

build-campfire: build-ui build-spark-bundles
	node scripts/prepare-ui.mjs
	go -C backend build -tags production -buildvcs=false -trimpath -ldflags '$(BUILD_LDFLAGS)' -o ../bin/campfire ./cmd/campfire
	go -C backend test -tags production -ldflags '$(BUILD_LDFLAGS)' ./internal/campfire/webui ./internal/campfire/sparkbundle

build-spark:
	go -C backend build -buildvcs=false -trimpath -ldflags '$(BUILD_LDFLAGS)' -o ../bin/spark ./cmd/spark

.PHONY: build-spark-bundles
build-spark-bundles:
	VERSION='$(VERSION)' COMMIT='$(COMMIT)' BUILD_DATE='$(BUILD_DATE)' bash scripts/build-spark-bundles.sh

build-ui:
	pnpm --dir web run build

test: test-backend test-frontend

test-backend:
	go -C backend test -race ./...

# Requires an existing disposable PostgreSQL database with CREATE SCHEMA access.
test-postgres:
	@test -n "$$CAMPFIRE_TEST_POSTGRES" || (echo "Set CAMPFIRE_TEST_POSTGRES to a PostgreSQL test URL"; exit 1)
	go -C backend test -race -count=1 ./internal/campfire/database/repository

# Check frontend types and installation command behavior.
test-frontend:
	pnpm --dir web run typecheck
	node --test scripts/test-spark-installer.mjs scripts/test-api-client.mjs scripts/test-operation-message.mjs scripts/test-console-stream.mjs scripts/test-resource-links.mjs

format: format-backend format-frontend

format-backend:
	gofmt -w backend

format-frontend:
	pnpm --dir web run format

format-check: format-check-backend format-check-frontend

format-check-backend:
	test -z "$$(gofmt -l backend)"

format-check-frontend:
	pnpm --dir web run format:check

lint: lint-backend lint-frontend

lint-backend: $(GOLANGCI_LINT)
	cd backend && $(GOLANGCI_LINT) run --config ../.golangci.yml ./...

lint-frontend:
	pnpm --dir web run lint

vet:
	go -C backend vet ./...

check: format-check vet lint test build

tools: $(GOLANGCI_LINT)

$(GOLANGCI_LINT):
	GOBIN=$(CURDIR)/.bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

dev:
	node scripts/dev.mjs

dev-mock:
	node scripts/dev.mjs --mock

dev-ui:
	pnpm --dir web run dev

preview:
	pnpm --dir web run preview
