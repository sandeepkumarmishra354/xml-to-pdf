BINARY  := xml2pdf
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: build test vet examples dist clean

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/xml2pdf

test:
	go test ./...

vet:
	gofmt -l . | (! grep .) && go vet ./...

examples: build
	bin/$(BINARY) examples/invoice.xml -data examples/invoice.json -o bin/invoice.pdf
	bin/$(BINARY) examples/report.xml -o bin/report.pdf

# Cross-compile static binaries for every platform into dist/.
dist:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" \
			-o dist/$(BINARY)-$$os-$$arch$$ext ./cmd/xml2pdf || exit 1; \
	done

clean:
	rm -rf bin dist
