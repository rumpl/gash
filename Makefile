.PHONY: all build fmt fmt-check test clean

all: test build

build:
	mkdir -p bin
	go build -trimpath -o bin/gash ./cmd/gash

fmt:
	gofumpt -w .

fmt-check:
	test -z "$$(gofumpt -l .)"
	! grep -REn '^func[^[:cntrl:]]*\{.*\}' --include='*.go' .

test: fmt-check
	go test ./...
	go test -race ./...
	go vet ./...

clean:
	rm -rf bin
