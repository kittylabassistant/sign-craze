VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X github.com/kittylabassistant/sign-craze/internal/version.Version=$(VERSION)
DIST    := dist

TARGETS := arm64 arm7 mipsle mips

# Внутри podman-контейнера (bazzite/Kinoite) используем host-spawn для доступа к хостовому podman.
# На хосте или в CI используем docker/podman напрямую.
ifeq ($(shell command -v host-spawn 2>/dev/null),)
  CONTAINER_RT ?= $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null || echo podman)
else
  CONTAINER_RT ?= host-spawn podman
endif

GO_IMAGE   := golang:1.25
WORKSPACE  := $(shell pwd)
GO_RUN     := $(CONTAINER_RT) run --rm -v $(WORKSPACE):/workspace:z -w /workspace $(GO_IMAGE)

.PHONY: all upx lint test test-integration tidy clean

all: $(TARGETS:%=$(DIST)/sign-craze-%)

$(DIST):
	mkdir -p $(DIST)

$(DIST)/sign-craze-arm64: $(DIST)
	$(CONTAINER_RT) run --rm -v $(WORKSPACE):/workspace:z -w /workspace \
	  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=arm64 \
	  $(GO_IMAGE) go build -ldflags="$(LDFLAGS)" -trimpath -o $@ ./cmd/sign-craze

$(DIST)/sign-craze-arm7: $(DIST)
	$(CONTAINER_RT) run --rm -v $(WORKSPACE):/workspace:z -w /workspace \
	  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=arm -e GOARM=7 \
	  $(GO_IMAGE) go build -ldflags="$(LDFLAGS)" -trimpath -o $@ ./cmd/sign-craze

$(DIST)/sign-craze-mipsle: $(DIST)
	$(CONTAINER_RT) run --rm -v $(WORKSPACE):/workspace:z -w /workspace \
	  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=mipsle -e GOMIPS=softfloat \
	  $(GO_IMAGE) go build -ldflags="$(LDFLAGS)" -trimpath -o $@ ./cmd/sign-craze

$(DIST)/sign-craze-mips: $(DIST)
	$(CONTAINER_RT) run --rm -v $(WORKSPACE):/workspace:z -w /workspace \
	  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=mips -e GOMIPS=softfloat \
	  $(GO_IMAGE) go build -ldflags="$(LDFLAGS)" -trimpath -o $@ ./cmd/sign-craze

upx: all
	upx --lzma $(DIST)/sign-craze-*

tidy:
	$(GO_RUN) go mod tidy

lint:
	$(CONTAINER_RT) run --rm -v $(WORKSPACE):/workspace:z -w /workspace \
	  golangci/golangci-lint:latest golangci-lint run ./...

test:
	$(GO_RUN) go test -race ./...

test-integration:
	$(CONTAINER_RT) build -t sign-craze-iptables-test -f testdata/docker/Dockerfile.iptables .
	$(CONTAINER_RT) run --privileged --rm sign-craze-iptables-test

release: all upx
	cd $(DIST) && sha256sum sign-craze-* > sha256sums.txt
	@echo "Release artifacts in $(DIST)/"

clean:
	rm -rf $(DIST)
