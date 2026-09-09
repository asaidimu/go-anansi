.PHONY: all build test test-ts test-all version vet

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
RELEASE ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")

# mattn/go-sqlite3 only compiles the FTS5 module with the sqlite_fts5 tag.
# Full-text indexes (IndexTypeFullText) and TextSearch queries fail at runtime
# without it ("no such module: fts5"), so every Go command below carries it.
GOTAGS ?= sqlite_fts5

all: vet test build

build:
	go build -tags "$(GOTAGS)" -v -ldflags "-X main.Version=$(VERSION) -X main.Release=$(RELEASE)" ./cmd/anansi

test:
	ANANSI_ENV=development go clean -testcache && ANANSI_ENV=development go test -tags "$(GOTAGS)" -v ./...

test-ts:
	cd packages/anansi && bun test

test-all: test test-ts

vet:
	go vet -tags "$(GOTAGS)" ./...

version:
	@echo $(VERSION)

release:
	@echo $(RELEASE)
