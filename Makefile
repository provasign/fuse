BINARY     := fuse
VERSION    ?= 0.1.0-dev
PKG        := github.com/provasign/fuse
LDFLAGS    := -X $(PKG)/internal/version.Version=$(VERSION)

.PHONY: build test lint install clean run release proto fmt

build:
	go build -ldflags="$(LDFLAGS)" -o bin/$(BINARY) ./cmd/fuse

test:
	go test ./...

test-race:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -20

lint:
	go vet ./...
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; fi

fmt:
	gofmt -w .
	go mod tidy

install: build
	install -m 0755 bin/$(BINARY) $(GOPATH)/bin/$(BINARY)

run: build
	./bin/$(BINARY) $(ARGS)

clean:
	rm -rf bin/ coverage.out

release:
	@command -v goreleaser >/dev/null 2>&1 || { echo "goreleaser not installed"; exit 1; }
	goreleaser release --snapshot --clean
