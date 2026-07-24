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
	rm -rf bin web/gash.wasm web/wasm_exec.js

.PHONY: wasm serve-wasm

wasm:
	mkdir -p web
	GOOS=js GOARCH=wasm go build -trimpath -o web/gash.wasm ./cmd/gash-wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js

serve-wasm: wasm
	python3 -m http.server 8080 --directory web
