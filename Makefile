VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X github.com/kittylabassistant/sign-craze/internal/version.Version=$(VERSION)
DIST    := dist

TARGETS := arm64 arm7 mipsle mips

.PHONY: all upx lint test test-integration clean

all: $(TARGETS:%=$(DIST)/sign-craze-%)

$(DIST):
	mkdir -p $(DIST)

$(DIST)/sign-craze-arm64: $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
	  go build -ldflags="$(LDFLAGS)" -trimpath -o $@ ./cmd/sign-craze

$(DIST)/sign-craze-arm7: $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 \
	  go build -ldflags="$(LDFLAGS)" -trimpath -o $@ ./cmd/sign-craze

$(DIST)/sign-craze-mipsle: $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat \
	  go build -ldflags="$(LDFLAGS)" -trimpath -o $@ ./cmd/sign-craze

$(DIST)/sign-craze-mips: $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=mips GOMIPS=softfloat \
	  go build -ldflags="$(LDFLAGS)" -trimpath -o $@ ./cmd/sign-craze

upx: all
	upx --lzma $(DIST)/sign-craze-*

lint:
	golangci-lint run ./...

test:
	go test -race ./...

test-integration:
	docker build -t sign-craze-iptables-test -f testdata/docker/Dockerfile.iptables .
	docker run --privileged --rm sign-craze-iptables-test

release: all upx
	cd $(DIST) && sha256sum sign-craze-* > sha256sums.txt
	@echo "Release artifacts in $(DIST)/"

clean:
	rm -rf $(DIST)
