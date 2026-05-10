BIN ?= bin/avm
VERSION ?= 0.0.0-dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build build-ui build-all test fmt vet clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/avm

build-ui:
	cd ui && npm ci && npm run typecheck && npm run build

build-all: build build-ui

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

clean:
	rm -rf bin dist ui/dist coverage.out
