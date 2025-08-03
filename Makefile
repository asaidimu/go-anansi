.PHONY: all build test

all: build

build:
	go build -v ./...

test:
	go test -v ./...
