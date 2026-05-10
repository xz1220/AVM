BIN ?= bin/avm
VERSION ?= 0.0.0-dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
NPM ?= npm
NPM_CI_FLAGS ?= --no-audit --no-fund --progress=false
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build build-ui build-all test fmt vet clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/avm

build-ui:
	cd ui && ($(NPM) ci $(NPM_CI_FLAGS) || (rm -rf node_modules && $(NPM) cache verify && $(NPM) ci $(NPM_CI_FLAGS))) && $(NPM) run typecheck && $(NPM) run build

build-all: build build-ui

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

clean:
	rm -rf bin dist ui/dist coverage.out
