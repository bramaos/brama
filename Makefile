# Brama development tasks.
#
# `make` with no target runs the same checks CI runs, in the same order.
# If `make check` is green locally, CI should be green too.

GO ?= go
BIN := bin/brama
PKG := ./...

# The shim is cross-built and embedded into brama, which is why `binary` depends on
# it. A plain `go build` still works without these: internal/shim/bin ships a
# placeholder so a checkout compiles, and a brama built that way says so when asked
# to install one.
SHIM_DIR := internal/shim/bin
SHIM_PLATFORMS := linux/amd64 linux/arm64

# Stamped into main.version at link time. Falls back to "dev" outside a git tree.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)
# The shim carries the same version as the brama that embeds it: brama compares what
# it shipped against what answers on the server, so the two come from one build.
SHIM_LDFLAGS := -s -w -X main.version=$(VERSION)

# Pinned to match .github/workflows/lint.yml. golangci-lint adds linters and tightens
# existing ones between minors, so a floating version fails on a commit that changed
# nothing. Bump both together.
GOLANGCI_VERSION ?= v2.13.2
GOLANGCI := $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
GOVULNCHECK := $(GO) run golang.org/x/vuln/cmd/govulncheck@latest

.DEFAULT_GOAL := check

## check: run every CI check, in CI's order
#
# shim comes before test on purpose: the tests that check an embedded build is for
# the architecture it is named for skip when there is nothing embedded, so running
# them first would report green for a matrix that was never built.
#
# vuln is last because it is the only check that reports on the outside world rather
# than on the tree: it can go red on a commit that changed nothing.
.PHONY: check
check: fmt-check tidy-check vet lint build shim test binary vuln

## build: compile every package
.PHONY: build
build:
	$(GO) build $(PKG)

## shim: cross-build the shim binaries brama embeds
.PHONY: shim
shim:
	@for platform in $(SHIM_PLATFORMS); do \
		os=$${platform%%/*}; arch=$${platform#*/}; \
		echo "  $(SHIM_DIR)/shim-$$os-$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath \
			-ldflags "$(SHIM_LDFLAGS)" \
			-o $(SHIM_DIR)/shim-$$os-$$arch ./cmd/brama-shim || exit 1; \
	done

## binary: build the brama binary into bin/, shims embedded
.PHONY: binary
binary: shim
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/brama

## test: run the test suite with the race detector
.PHONY: test
test:
	$(GO) test -race $(PKG)

## vet: report suspicious constructs
.PHONY: vet
vet:
	$(GO) vet $(PKG)

## lint: run golangci-lint
.PHONY: lint
lint:
	$(GOLANGCI) run $(PKG)

## lint-fix: apply the fixes golangci-lint can make on its own
.PHONY: lint-fix
lint-fix:
	$(GOLANGCI) run --fix $(PKG)
	$(GOLANGCI) fmt

## vuln: report known vulnerabilities in dependencies
.PHONY: vuln
vuln:
	$(GOVULNCHECK) $(PKG)

## fmt: rewrite source files to gofmt style
.PHONY: fmt
fmt:
	gofmt -w -l .

## fmt-check: fail if any source file is not gofmt'd
.PHONY: fmt-check
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Not gofmt'd:"; \
		echo "$$unformatted"; \
		echo "Run 'make fmt'."; \
		exit 1; \
	fi

## tidy-check: fail if go.mod or go.sum are out of date
.PHONY: tidy-check
tidy-check:
	$(GO) mod tidy -diff

## clean: remove build artifacts
.PHONY: clean
clean:
	rm -rf bin/
	rm -f $(SHIM_DIR)/shim-*

## help: list available targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
