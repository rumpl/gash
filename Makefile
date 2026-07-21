.PHONY: all build test clean

all: test build

build:
	mkdir -p bin
	go build -trimpath -o bin/gash ./cmd/gash

test:
	go test ./...
	go test -race ./...
	go vet ./...

clean:
	rm -rf bin
