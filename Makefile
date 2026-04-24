BIN ?= bin/avm

.PHONY: build test fmt vet clean

build:
	go build -o $(BIN) ./cmd/avm

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

clean:
	rm -rf bin dist coverage.out
