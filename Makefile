.PHONY: all build fmt fmt-check test clean

all: test build

build:
	mkdir -p bin
	go build -trimpath -o bin/gash ./cmd/gash

fmt:
	gofumpt -w .

fmt-check:
	test -z "$$(gofumpt -l .)"
	! grep -REn '^func[^[:cntrl:]]*\{.*\}' --include='*.go' --exclude-dir=third_party .

test: fmt-check
	go test ./...
	go test -race ./...
	go vet ./...

clean:
	rm -rf bin web/gash.wasm web/wasm_exec.js

.PHONY: wasm test-wasm serve-wasm

wasm:
	mkdir -p web
	GOOS=js GOARCH=wasm go build -trimpath -o web/gash.wasm ./cmd/gash-wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js

test-wasm:
	mkdir -p bin
	GOOS=js GOARCH=wasm go test -c -o bin/gash-wasm.test ./pkg/gash
	"$$(go env GOROOT)/lib/wasm/go_js_wasm_exec" bin/gash-wasm.test -test.run 'Wasm'

serve-wasm: wasm
	python3 -m http.server 8080 --directory web
