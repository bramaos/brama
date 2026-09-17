# Brama development tasks.
#
# `make` with no target runs the same checks CI runs, in the same order.
# If `make check` is green locally, CI should be green too — and that is a fact
# rather than a hope, because CI runs these targets. The workflows in
# .github/workflows call `make fmt-check`, `make lint`, `make test` and the rest
# instead of spelling the commands out a second time, so a recipe can only drift
# from what CI runs by being edited here, where the change is visible.
#
# Tool versions are pinned here too, once. The workflows read them back out with
# `make print-GOLANGCI_VERSION`, so there is no second copy to keep in step.

GO ?= go
BIN := bin/brama
PKG := ./...

# Written by `make cover`, read by `make cover-html` and uploaded by CI. atomic is
# the only counting mode that is correct under -race, which every test run uses.
COVERPROFILE := coverage.out
COVERMODE    := atomic

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

# The tool pins. These are the only copies: the workflows ask for them with
# `make print-GOLANGCI_VERSION`, so bumping a version here bumps it in CI too.
#
# Pinned rather than floating because golangci-lint adds linters and tightens
# existing ones between minors, so `latest` fails on a commit that changed nothing.
GOLANGCI_VERSION ?= v2.13.2
GOLANGCI := $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
GOVULNCHECK_VERSION ?= v1.8.0
GOVULNCHECK := $(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
# goreleaser builds the release. Pinned so a snapshot built on a laptop and the
# artifacts built from a tag come off the same tool.
GORELEASER_VERSION ?= v2.18.1
GORELEASER := $(GO) run github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)

.DEFAULT_GOAL := check

## check: run every CI check, in CI's order
#
# shim comes before test on purpose: the tests that check an embedded build is for
# the architecture it is named for skip when there is nothing embedded, so running
# them first would report green for a matrix that was never built.
#
# vuln is last because it is the only check that reports on the outside world rather
# than on the tree: it can go red on a commit that changed nothing.
#
# cover rather than test: the profile is free once -race is already running, and
# running the same target CI runs is what keeps the sentence at the top of this file
# true. `make test` is still there for the fast path.
.PHONY: check
check: fmt-check tidy-check vet lint build shim cover binary vuln

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

## cover: run the test suite and write a coverage profile
#
# Depends on shim for the same reason check orders it that way: the tests that read
# an embedded build skip when there is nothing embedded, and a skipped test is
# counted as uncovered, so a profile taken without the shims understates coverage.
.PHONY: cover
cover: shim
	$(GO) test -race -covermode=$(COVERMODE) -coverprofile=$(COVERPROFILE) $(PKG)
	@$(GO) tool cover -func=$(COVERPROFILE) | tail -1

## cover-html: open the coverage profile as an annotated source listing
.PHONY: cover-html
cover-html: cover
	$(GO) tool cover -html=$(COVERPROFILE) -o coverage.html
	@echo "  coverage.html"

## bench: run the benchmarks
.PHONY: bench
bench:
	$(GO) test -run '^$$' -bench . -benchmem $(PKG)

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

## run: build brama and run it, shims embedded (make run ARGS="server add ...")
.PHONY: run
run: binary
	@$(BIN) $(ARGS)

## install: install brama into GOBIN, shims embedded
.PHONY: install
install: shim
	$(GO) install -ldflags "$(LDFLAGS)" ./cmd/brama

## release-check: validate .goreleaser.yaml without building anything
.PHONY: release-check
release-check:
	$(GORELEASER) check

## release-snapshot: build the full release locally, into dist/, without publishing
#
# The same command the release workflow runs, minus the tag and the upload. Run it
# before tagging: a release that only fails once the tag exists is a release that
# has to be un-tagged.
#
# sbom is skipped because it needs syft, which the workflow installs and a laptop
# generally has not. What is being rehearsed here is the build, not the packaging.
.PHONY: release-snapshot
release-snapshot:
	$(GORELEASER) release --snapshot --clean --skip=publish,sbom

## release-notes: print the CHANGELOG section for a tag (make release-notes TAG=v0.1.0)
#
# What the release workflow publishes as the release body. CHANGELOG.md is written by
# hand and is this project's record of what changed; generating notes from commit
# subjects instead would publish a second, worse version of it.
.PHONY: release-notes
release-notes:
	@test -n "$(TAG)" || { echo "usage: make release-notes TAG=v0.1.0" >&2; exit 1; }
	@awk -v v='$(TAG:v%=%)' ' \
		$$0 ~ "^## \\[" v "\\]" { found = 1; next } \
		found && /^## \[/ { exit } \
		found { print } \
		END { if (!found) { print "no CHANGELOG.md section for " v > "/dev/stderr"; exit 1 } } \
	' CHANGELOG.md

# --- Containerised Servers ------------------------------------------------------
#
# A rig of two real Servers — Ubuntu, systemd, sshd, MariaDB — that brama reaches
# through the `ssh` binary exactly as it reaches a rented Server. None of it is
# wired into `check`, `test` or CI: those stay fast and need no container, and the
# tests below need one. See test/testenv/README.md.

TESTENV_DIR     := test/testenv
TESTENV_COMPOSE := docker compose -f $(TESTENV_DIR)/compose.yml
TESTENV_SERVERS := production staging
# The images the Dockerfile builds FROM. Named here because they are pulled before
# the build rather than during it — see testenv-pull.
TESTENV_IMAGES  := ubuntu:24.04 composer:2.8

# The published ports, defined once and exported, because two consumers have to agree
# on them: compose publishes them, and ssh-setup.sh writes them into the config
# fragment. Left to default independently, a change to one would produce a fragment
# pointing at a port nothing listens on.
export PRODUCTION_SSH_PORT  ?= 2201
export PRODUCTION_HTTP_PORT ?= 8081
export STAGING_SSH_PORT     ?= 2202
export STAGING_HTTP_PORT    ?= 8082

## testenv-ssh-setup: generate the rig's keypair and ssh config fragment
#
# Prints the Include line to paste. Writes nothing to ~/.ssh/config.
.PHONY: testenv-ssh-setup
testenv-ssh-setup:
	@$(TESTENV_DIR)/bin/ssh-setup.sh

## testenv-up: build the image and start both Servers
#
# Depends on the keypair: the public key is mounted into both containers, and Docker
# would otherwise silently create a directory where the file should be.
.PHONY: testenv-up
testenv-up: testenv-ssh-setup testenv-pull
	$(TESTENV_COMPOSE) up -d --build --wait
	@echo
	@$(TESTENV_COMPOSE) ps

## testenv-pull: fetch the base images the rig builds from
#
# Separate from the build, and before it, because buildx resolves `FROM` through the
# client's credential helper while `docker pull` goes through the daemon. On a Docker
# Desktop WSL setup the former is docker-credential-desktop.exe, which WSL cannot
# execute, so a build that has to reach the registry fails on an image that is
# public. Pulling first leaves nothing for the build to resolve.
.PHONY: testenv-pull
testenv-pull:
	@for image in $(TESTENV_IMAGES); do \
		echo "  $$image"; docker pull -q $$image >/dev/null || exit 1; \
	done

## testenv-down: stop both Servers and remove them
.PHONY: testenv-down
testenv-down:
	$(TESTENV_COMPOSE) down --remove-orphans
	@rm -f $(TESTENV_DIR)/ssh/known_hosts

## testenv-seed: load the deterministic WordPress data into both Servers
#
# Also the reset: it drops what is there first, so a Server broken by hand comes back
# to a known state without a rebuild.
.PHONY: testenv-seed
testenv-seed:
	@for server in $(TESTENV_SERVERS); do \
		echo "==> seeding $$server"; \
		$(TESTENV_COMPOSE) exec -T $$server \
			/usr/local/share/brama-testenv/seed/seed.sh || exit 1; \
	done

## testenv-test: run the tests that need a running rig
#
# Build-tagged, so `make test` and `make check` never compile them. binary first:
# these tests install the embedded Shim, and a brama built without one has nothing
# to install.
.PHONY: testenv-test
testenv-test: shim
	$(GO) test -tags testenv -count=1 -v ./$(TESTENV_DIR)/...

## testenv-shell: open a root shell on a Server (make testenv-shell SERVER=staging)
.PHONY: testenv-shell
testenv-shell:
	@$(TESTENV_COMPOSE) exec $(or $(SERVER),production) bash

## clean: remove build artifacts
.PHONY: clean
clean:
	rm -rf bin/ dist/
	rm -f $(SHIM_DIR)/shim-* $(COVERPROFILE) coverage.html

## help: list available targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'

# print-<VAR>: echo the value of a Makefile variable.
#
# How CI reads the tool pins above without holding a second copy of them. Not listed
# in help: it is plumbing for the workflows, not a task anyone runs by hand.
.PHONY: print-%
print-%:
	@echo '$($*)'
