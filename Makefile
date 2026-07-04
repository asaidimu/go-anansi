.PHONY: all build test

all: build

build:
	go build -v ./...

test:
	ANANSI_ENV=development go clean -testcache && ANANSI_ENV=development go test -v ./...
