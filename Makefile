.PHONY: all build test version

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
RELEASE ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")

all: build

build:
	go build -v -ldflags "-X main.Version=$(VERSION) -X main.Release=$(RELEASE)" ./cmd/anansi

test:
	ANANSI_ENV=development go clean -testcache && ANANSI_ENV=development go test -v ./...

version:
	@echo $(VERSION)

release:
	@echo $(RELEASE)
