GO ?= go
PYTHON ?= python3

.PHONY: fmt
fmt:
	$(GO) fmt ./...

.PHONY: test-go
test-go:
	$(GO) test ./...

.PHONY: test-worker
test-worker:
	cd worker && $(PYTHON) -m pytest tests

.PHONY: test
test: test-go test-worker

.PHONY: build
build:
	$(GO) build -o ./bin/voxi ./cmd/voxi

.PHONY: run-daemon
run-daemon:
	$(GO) run ./cmd/voxi daemon
