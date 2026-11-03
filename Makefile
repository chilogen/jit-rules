VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null)
LDFLAGS = -X github.com/chilogen/jit-rules/internal/cli.Version=$(VERSION) -X github.com/chilogen/jit-rules/internal/cli.Commit=$(COMMIT)

.PHONY: bootstrap build install test lint check dist

bootstrap:
	sh scripts/bootstrap.sh

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/jitr ./cmd/jitr

# install replaces the jitr at the stable path that installed hooks call
install: build
	bin/jitr upgrade bin/jitr

test:
	go test ./...

lint:
	go vet ./...
	@files="$$(gofmt -l .)"; if [ -n "$$files" ]; then printf '%s\n' "$$files"; exit 1; fi

check: lint test

dist:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/jitr-linux-amd64 ./cmd/jitr
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/jitr-linux-arm64 ./cmd/jitr
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/jitr-darwin-amd64 ./cmd/jitr
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/jitr-darwin-arm64 ./cmd/jitr
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/jitr-windows-amd64.exe ./cmd/jitr
