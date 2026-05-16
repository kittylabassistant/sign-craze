VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -buildid= -X github.com/kittylabassistant/sign-craze/internal/version.Version=$(VERSION)
DIST    := dist

TARGETS := arm64 arm7 mipsle mips

# mieru — supervised peer (ADR-0020). Собирается отдельно от sign-craze:
# отдельный go install, не импортируем mieru-пакеты в основной binary.
# Apply ldflags="-s -w -trimpath" → ~7-15 МБ raw, ~3-6 МБ UPX (arm/arm64).
MIERU_VERSION ?= v3.32.0

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

.PHONY: all upx lint test test-integration tidy clean bundle mieru mieru-upx

all: $(TARGETS:%=$(DIST)/sign-craze-%)

# mieru: собирает mieru-клиент под все 4 архитектуры. Используется отдельно
# от `all` — пользователю надо явно вызвать `make mieru` чтобы скачать +
# скомпилировать mieru. ADR-0020 разрешает не включать mieru в release-bundle
# по умолчанию, но bundle-mieru target ниже добавляет его в tarballs.
mieru: $(TARGETS:%=$(DIST)/mieru-%)

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

# ─── mieru cross-compile (ADR-0020) ──────────────────────────────────────────
#
# Каждый target собирает mieru/cmd/mieru как pure-Go бинарь под нужный
# GOOS/GOARCH/GOMIPS. mieru-пакеты НЕ импортируются в основной sign-craze
# binary (ADR-0020): отдельный `go install pkg@version` в изолированном
# tmp-каталоге.
#
# Module-path mieru: с v3.0 это `github.com/enfein/mieru/v3` (модуль с
# major-version suffix). Параметр MIERU_VERSION (по умолчанию v3.32.0) должен
# соответствовать v3.x.x.
#
# `go install pkg@version` в cross-compile режиме кладёт результат в
# $GOBIN/<goos>_<goarch>/mieru (если бинарь native — в $GOBIN/mieru).
# После сборки переименовываем в `mieru-<arch>` для распознавания install.sh.

MIERU_PKG := github.com/enfein/mieru/v3/cmd/mieru
MIERU_GOCACHE := $(WORKSPACE)/$(DIST)/.gocache-mieru

$(DIST)/mieru-arm64: $(DIST)
	@mkdir -p $(MIERU_GOCACHE)
	$(CONTAINER_RT) run --rm \
	  -v $(WORKSPACE):/workspace:z -w /workspace \
	  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=arm64 \
	  -e GOPATH=/workspace/$(DIST)/.gocache-mieru \
	  -e GOMODCACHE=/workspace/$(DIST)/.gocache-mieru/mod \
	  -e GOCACHE=/workspace/$(DIST)/.gocache-mieru/build \
	  -e GOFLAGS=-trimpath \
	  $(GO_IMAGE) sh -c 'go install -ldflags="-s -w" $(MIERU_PKG)@$(MIERU_VERSION) && \
	    cp /workspace/$(DIST)/.gocache-mieru/bin/linux_arm64/mieru /workspace/$@'

$(DIST)/mieru-arm7: $(DIST)
	@mkdir -p $(MIERU_GOCACHE)
	$(CONTAINER_RT) run --rm \
	  -v $(WORKSPACE):/workspace:z -w /workspace \
	  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=arm -e GOARM=7 \
	  -e GOPATH=/workspace/$(DIST)/.gocache-mieru \
	  -e GOMODCACHE=/workspace/$(DIST)/.gocache-mieru/mod \
	  -e GOCACHE=/workspace/$(DIST)/.gocache-mieru/build \
	  -e GOFLAGS=-trimpath \
	  $(GO_IMAGE) sh -c 'go install -ldflags="-s -w" $(MIERU_PKG)@$(MIERU_VERSION) && \
	    cp /workspace/$(DIST)/.gocache-mieru/bin/linux_arm/mieru /workspace/$@'

$(DIST)/mieru-mipsle: $(DIST)
	@mkdir -p $(MIERU_GOCACHE)
	$(CONTAINER_RT) run --rm \
	  -v $(WORKSPACE):/workspace:z -w /workspace \
	  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=mipsle -e GOMIPS=softfloat \
	  -e GOPATH=/workspace/$(DIST)/.gocache-mieru \
	  -e GOMODCACHE=/workspace/$(DIST)/.gocache-mieru/mod \
	  -e GOCACHE=/workspace/$(DIST)/.gocache-mieru/build \
	  -e GOFLAGS=-trimpath \
	  $(GO_IMAGE) sh -c 'go install -ldflags="-s -w" $(MIERU_PKG)@$(MIERU_VERSION) && \
	    cp /workspace/$(DIST)/.gocache-mieru/bin/linux_mipsle/mieru /workspace/$@'

$(DIST)/mieru-mips: $(DIST)
	@mkdir -p $(MIERU_GOCACHE)
	$(CONTAINER_RT) run --rm \
	  -v $(WORKSPACE):/workspace:z -w /workspace \
	  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=mips -e GOMIPS=softfloat \
	  -e GOPATH=/workspace/$(DIST)/.gocache-mieru \
	  -e GOMODCACHE=/workspace/$(DIST)/.gocache-mieru/mod \
	  -e GOCACHE=/workspace/$(DIST)/.gocache-mieru/build \
	  -e GOFLAGS=-trimpath \
	  $(GO_IMAGE) sh -c 'go install -ldflags="-s -w" $(MIERU_PKG)@$(MIERU_VERSION) && \
	    cp /workspace/$(DIST)/.gocache-mieru/bin/linux_mips/mieru /workspace/$@'

# mieru-upx: сжатие mieru-бинарей. mips/mipsle пропускаются (UPX-стаб
# несовместим с Keenetic 4.9, см. инцидент 2026-05-07 в tasks/lessons.md).
mieru-upx: mieru
	upx --lzma $(DIST)/mieru-arm64 $(DIST)/mieru-arm7 || true
	@echo "skip upx for mieru mipsle/mips: UPX-стаб несовместим с Keenetic 4.9"

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
