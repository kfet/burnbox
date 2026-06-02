.PHONY: all build build-matrix build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 fmt vet lint-frontend run-tests open_coverage clean e2e release-local _all

# Version metadata baked into the binary at link time. Override on the
# command line for reproducible release builds: `make build VERSION=v0.1.1`.
VERSION    ?= $(shell cat VERSION 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w \
	-X github.com/kfet/burnbox.Version=$(VERSION) \
	-X github.com/kfet/burnbox.Commit=$(COMMIT) \
	-X github.com/kfet/burnbox.BuildDate=$(BUILD_DATE)

# Quiet runner: $(call RUN,label,cmd) — runs cmd silently, prints "✓ label"
# on success, dumps captured output and exits non-zero on failure. V=1 for
# verbose.
ifdef V
  define RUN
	@echo "→ $(1)"
	@$(2)
  endef
else
  define RUN
	@_log=$$(mktemp); \
	if ( $(2) ) > $$_log 2>&1; then \
		echo "✓ $(1)"; rm -f $$_log; \
	else \
		rc=$$?; cat $$_log; rm -f $$_log; exit $$rc; \
	fi
  endef
endif

# Default target. Build + cross-compile matrix + gofmt + vet + frontend
# lint + race/shuffle tests with a 100% coverage gate + the e2e smoke,
# fanned out via a recursive `make -j`.
all:
	@$(MAKE) -j --no-print-directory _all
	@echo "✓ all green"

_all: build build-matrix fmt vet lint-frontend run-tests e2e

build:
	$(call RUN,build ./burnbox,go build -trimpath -ldflags='$(LDFLAGS)' -o burnbox ./cmd/burnbox)

# Cross-compile check (compile-only, no artefacts). CGO disabled; pure Go.
build-matrix: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64

build-linux-amd64:
	$(call RUN,build linux/amd64,CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 go build -trimpath -ldflags='$(LDFLAGS)' -o /dev/null ./cmd/burnbox)
build-linux-arm64:
	$(call RUN,build linux/arm64,CGO_ENABLED=0 GOOS=linux  GOARCH=arm64 go build -trimpath -ldflags='$(LDFLAGS)' -o /dev/null ./cmd/burnbox)
build-darwin-amd64:
	$(call RUN,build darwin/amd64,CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags='$(LDFLAGS)' -o /dev/null ./cmd/burnbox)
build-darwin-arm64:
	$(call RUN,build darwin/arm64,CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags='$(LDFLAGS)' -o /dev/null ./cmd/burnbox)

# Build real, shippable per-arch binary tarballs (darwin/linux ×
# amd64/arm64) into dist/, plus dist/checksums.txt. This is the exact
# shape the CI release workflow uploads as release assets and feeds to
# the Homebrew formula renderer — run it locally to verify a release
# before tagging. Output is gitignored. Pass VERSION to stamp the build:
# `make release-local VERSION=0.1.2`.
release-local:
	@rm -rf dist && mkdir -p dist
	@for t in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do \
		os=$${t%/*}; arch=$${t#*/}; name="burnbox-$(VERSION)-$$os-$$arch"; \
		echo "→ $$name"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath \
			-ldflags='$(LDFLAGS)' -o dist/$$name/burnbox ./cmd/burnbox || exit $$?; \
		cp LICENSE README.md dist/$$name/ 2>/dev/null || true; \
		tar -C dist -czf dist/$$name.tar.gz $$name; \
		rm -rf dist/$$name; \
	done
	@cd dist && shasum -a 256 *.tar.gz > checksums.txt
	@echo "✓ release artefacts in dist/"
	@ls -l dist/

fmt:
	$(call RUN,gofmt,gofmt -l . | (! grep .))

vet:
	$(call RUN,go vet clean,go vet ./...)

# Static checks for the bundled HTML / JS / CSS assets. Pure-Go; if
# `node` is on PATH we also `node --check` each .js file, otherwise the
# JS parse pass is skipped with a notice.
lint-frontend:
	$(call RUN,frontend lint,GOCACHE=$(CURDIR)/.cache/lintfrontend go run ./scripts/lintfrontend ./internal/ui/assets)

# Unit tests: race + shuffle + fresh cache + 100% coverage gate.
run-tests:
	@go clean -testcache
	$(call RUN,tests pass,go test -race -shuffle=on -cover ./... -coverprofile=coverage.tmp.out)
	$(call RUN,coverage clean,go run github.com/kfet/covgate/cmd/covgate@v0.1.0 -profile=coverage.tmp.out -out=coverage.out -ignore=.covignore -min=100)
	@rm -f coverage.tmp.out

open_coverage:
	go tool cover -html=coverage.out

# End-to-end smoke: starts the server, does the v1 client crypto in Go,
# POSTs the blob, then decrypts via the REAL bare-OS recipient one-liner
# (python3 + openssl) and asserts the round-trip + burn.
e2e:
	$(call RUN,e2e smoke,E2E=1 go test -count=1 -timeout=60s ./e2e/...)

clean:
	rm -f coverage.out coverage.tmp.out burnbox
	rm -rf dist .cache
