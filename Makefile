BINARY  := mp3blaster
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo 3.2.6-go)
LDFLAGS := -s -w -X main.version=$(VERSION)
# purego (used by oto) needs this flag to build without cgo on FreeBSD/NetBSD;
# it is harmless on the other platforms.
GCFLAGS := github.com/ebitengine/purego/internal/fakecgo=-std
GOBUILD := go build -trimpath -gcflags="$(GCFLAGS)" -ldflags "$(LDFLAGS)"
TARGETS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64 freebsd/amd64

# Pure Go everywhere: audio output goes through oto without cgo.
export CGO_ENABLED := 0

.PHONY: all build test race vet fmt clean cross

all: vet test build

build:
	$(GOBUILD) -o bin/$(BINARY) ./cmd/mp3blaster

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
		GOOS=$$os GOARCH=$$arch $(GOBUILD) -o bin/$(BINARY)-$$os-$$arch$$ext ./cmd/mp3blaster || exit 1; \
	done

clean:
	rm -rf bin
