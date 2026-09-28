# Alfred's Go helper: the installer's bookkeeping and the agent generator.
#
# The Python scripts under scripts/ are still in the tree and still work. `make parity`
# is what says the two agree; it runs the real scripts and diffs their output byte for
# byte, and it is the check that has to stay green until the scripts are deleted.

GO ?= go
BIN := .build/alfred

.PHONY: all build test parity lint fmt vet check clean

all: check

build:
	$(GO) build -o $(BIN) ./cmd/alfred

test:
	$(GO) test ./...

parity:
	$(GO) test ./internal/parity/ -v -count=1

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
