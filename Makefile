# Alfred's Go helper: the installer's bookkeeping and the agent generator.
#
# `make golden` is the check that matters most: it pins the exact bytes the installer
# emits, which other programs read.

GO ?= go
BIN := .build/alfred

.PHONY: all build test golden lint fmt vet check clean

all: check

build:
	$(GO) build -o $(BIN) ./cmd/alfred

test:
	$(GO) test ./...

golden:
	$(GO) test ./internal/golden/ -v -count=1

lint:
	golangci-lint run ./...

fmt:
	gofmt -w cmd internal

vet:
	$(GO) vet ./...

# What has to pass before a change is done.
check: build vet lint test

clean:
	rm -rf .build
