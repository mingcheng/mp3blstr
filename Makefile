BINARY  := mp3blstr
VERSION ?= $(shell git describe --tags --exact-match --match 'v*' 2>/dev/null || echo dev)
BUILD_DATE ?= $(shell date -u +%Y%m%d)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.buildDate=$(BUILD_DATE)
# purego (used by oto) needs this flag to build without cgo on FreeBSD/NetBSD;
# it is harmless on the other platforms.
GCFLAGS := github.com/ebitengine/purego/internal/fakecgo=-std
GOBUILD := go build -trimpath -gcflags="$(GCFLAGS)" -ldflags "$(LDFLAGS)"
TARGETS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64 freebsd/amd64
PREFIX ?= /usr/local
DESTDIR ?=

# Pure Go everywhere: audio output goes through oto without cgo.
export CGO_ENABLED := 0

.PHONY: all build test race vet fmt clean cross install

all: vet test build

build:
	$(GOBUILD) -o bin/$(BINARY) ./cmd/mp3blstr

install: build
	install -Dm755 bin/$(BINARY) $(DESTDIR)$(PREFIX)/bin/$(BINARY)
	install -Dm644 share/applications/mp3blstr.desktop $(DESTDIR)$(PREFIX)/share/applications/mp3blstr.desktop

test:
	go test ./...

# The race detector requires cgo.
race:
	CGO_ENABLED=1 go test -race ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

cross:
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "build $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch $(GOBUILD) -o bin/$(BINARY)-$$os-$$arch$$ext ./cmd/mp3blstr || exit 1; \
	done

clean:
	rm -rf bin
