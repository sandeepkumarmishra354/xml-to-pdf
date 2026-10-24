BINARY  := xml2pdf
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: build test vet examples docs playground dist snapshot clean

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/xml2pdf

test:
	go test ./...

vet:
	gofmt -l . | (! grep .) && go vet ./...

examples: build
	bin/$(BINARY) examples/invoice.xml -data examples/invoice.json -o bin/invoice.pdf
	bin/$(BINARY) examples/report.xml -o bin/report.pdf

# Rebuild the static documentation site (docs/) from docs-src/, including
# fresh renders of the example documents.
docs: build
	bin/$(BINARY) examples/hello.xml -o docs/assets/hello.pdf
	bin/$(BINARY) examples/invoice.xml -data examples/invoice.json -o docs/assets/invoice.pdf
	bin/$(BINARY) examples/report.xml -o docs/assets/report.pdf
	@if command -v pdftoppm >/dev/null; then \
		for f in hello invoice report; do pdftoppm -r 80 -png docs/assets/$$f.pdf docs/assets/$$f; done; \
	else echo "pdftoppm not found: skipping PNG previews"; fi
	python3 docs-src/build.py

# Build the browser playground (docs/playground/) from the WebAssembly target.
# Needs Go and Node.js (for the CodeMirror bundle). Outputs are gitignored;
# the Pages workflow builds them on deploy.
# Preview locally with: python3 -m http.server -d docs
PLAYGROUND := docs/playground
playground:
	npm ci --prefix web/editor --no-audit --no-fund
	npm run --prefix web/editor build
	GOOS=js GOARCH=wasm go build -trimpath -ldflags "$(LDFLAGS)" -o $(PLAYGROUND)/xml2pdf.wasm ./cmd/xml2pdf-wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(PLAYGROUND)/
	mkdir -p $(PLAYGROUND)/examples
	cp examples/hello.xml examples/invoice.xml examples/invoice.json examples/loop.xml examples/loop.json examples/report.xml examples/logo.png $(PLAYGROUND)/examples/

# Cross-compile static binaries for every platform into dist/.
dist:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" \
			-o dist/$(BINARY)-$$os-$$arch$$ext ./cmd/xml2pdf || exit 1; \
	done

# Build release archives locally with GoReleaser (no publishing) into dist/.
snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist $(PLAYGROUND)/xml2pdf.wasm $(PLAYGROUND)/wasm_exec.js $(PLAYGROUND)/examples $(PLAYGROUND)/editor.js
