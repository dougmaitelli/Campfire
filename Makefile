GOLANGCI_LINT_VERSION := v2.12.2
GOLANGCI_LINT ?= $(CURDIR)/.bin/golangci-lint
GOLANGCI_LINT_CACHE := $(CURDIR)/.cache/golangci-lint
export GOLANGCI_LINT_CACHE

.PHONY: dependencies dependencies-backend dependencies-frontend \
	build build-backend build-frontend test test-backend test-frontend \
	format format-backend format-frontend format-check format-check-backend format-check-frontend \
	lint lint-backend lint-frontend vet check tools dev dev-ui preview \
	backend-build backend-test backend-vet ui-build ui-check ui-watch

dependencies: dependencies-backend dependencies-frontend

dependencies-backend:
	go -C backend mod download

dependencies-frontend:
	pnpm --dir web install --frozen-lockfile

build: build-backend build-frontend

build-backend:
	go -C backend build -buildvcs=false -trimpath -o ../bin/campfire ./cmd/campfire
	go -C backend build -buildvcs=false -trimpath -o ../bin/spark ./cmd/spark

build-frontend:
	pnpm --dir web run build

test: test-backend test-frontend

test-backend:
	go -C backend test -race ./...

# There is no application behavior to test yet; check frontend types for now.
test-frontend:
	pnpm --dir web run typecheck

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
	node --check scripts/dev.mjs

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

dev-ui:
	pnpm --dir web run dev

preview:
	pnpm --dir web run preview

# Keep the original skeleton commands as aliases.
backend-build: build-backend
backend-test: test-backend
backend-vet: vet
ui-build: build-frontend
ui-check: test-frontend
ui-watch: dev-ui
