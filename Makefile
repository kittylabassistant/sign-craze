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

.PHONY: all upx lint test test-integration tidy clean bundle

all: $(TARGETS:%=$(DIST)/sign-craze-%)

$(DIST):
	mkdir -p $(DIST)

$(DIST)/sign-craze-arm64: $(DIST)
	$(CONTAINER_RT) run --rm -v $(WORKSPACE):/workspace:z -w /workspace \
	  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=arm64 \
	  $(GO_IMAGE) go build -buildvcs=false -ldflags="$(LDFLAGS)" -trimpath -o $@ ./cmd/sign-craze

$(DIST)/sign-craze-arm7: $(DIST)
	$(CONTAINER_RT) run --rm -v $(WORKSPACE):/workspace:z -w /workspace \
	  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=arm -e GOARM=7 \
	  $(GO_IMAGE) go build -buildvcs=false -ldflags="$(LDFLAGS)" -trimpath -o $@ ./cmd/sign-craze

$(DIST)/sign-craze-mipsle: $(DIST)
	$(CONTAINER_RT) run --rm -v $(WORKSPACE):/workspace:z -w /workspace \
	  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=mipsle -e GOMIPS=softfloat \
	  $(GO_IMAGE) go build -buildvcs=false -ldflags="$(LDFLAGS)" -trimpath -o $@ ./cmd/sign-craze

$(DIST)/sign-craze-mips: $(DIST)
	$(CONTAINER_RT) run --rm -v $(WORKSPACE):/workspace:z -w /workspace \
	  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=mips -e GOMIPS=softfloat \
	  $(GO_IMAGE) go build -buildvcs=false -ldflags="$(LDFLAGS)" -trimpath -o $@ ./cmd/sign-craze

upx: all
	upx --lzma $(DIST)/sign-craze-arm64 $(DIST)/sign-craze-arm7
	@echo "skip upx for mipsle/mips: UPX-стаб крашит CLI на Keenetic 4.9"

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

# bundle: per-arch tar.gz для offline-установки.
# Содержит sign-craze, sign-craze.sha256, install.sh, install-offline.sh.
bundle: all upx
	@for arch in $(TARGETS); do \
	  B=$(DIST)/signcraze-$$arch-bundle; \
	  rm -rf $$B && mkdir -p $$B; \
	  cp $(DIST)/sign-craze-$$arch $$B/sign-craze; \
	  (cd $$B && sha256sum sign-craze > sign-craze.sha256); \
	  cp scripts/install.sh $$B/install.sh; \
	  printf '#!/bin/sh\nset -eu\nDIR=$$(cd "$$(dirname "$$0")" && pwd)\nexec env SIGNCRAZE_BIN="$$DIR/sign-craze" SIGNCRAZE_SHA256="$$DIR/sign-craze.sha256" SIGNCRAZE_VERSION="$(VERSION)-'$$arch'" sh "$$DIR/install.sh"\n' > $$B/install-offline.sh; \
	  chmod +x $$B/install-offline.sh; \
	  tar czf $(DIST)/signcraze-$$arch.tar.gz -C $(DIST) signcraze-$$arch-bundle; \
	  rm -rf $$B; \
	done
	@echo "Bundles in $(DIST)/signcraze-*.tar.gz"

clean:
	rm -rf $(DIST)
