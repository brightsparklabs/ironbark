##
 # Makefile
 # ______________________________________________________________________________
 #
 # Created by brightSPARK Labs
 # www.brightsparklabs.com
 ##

# Warn whenever make sees a reference to an undefined variable.
MAKEFLAGS += --warn-undefined-variables
# Disable implicit rules as they are not needed by this project.
MAKEFLAGS += --no-builtin-rules
# Set the shell to `bash` to give better error messaging/handling.
# See https://stackoverflow.com/questions/20615217/bash-bad-substitution
SHELL := bash

# Use `:=` (immediate evaluation) so every `$(shell ...)` is invoked exactly
# once when the Makefile is parsed. This guarantees that all derived values
# (e.g. `GO_LDFLAGS`) see consistent build metadata.

# Limit CPU usage for GoReleaser builds to prevent CPU saturation (can be overridden: GOMAXPROCS=4 make release-build)
GOMAXPROCS ?= $(shell echo $$(( $$(nproc) / 2 )))

APP_NAME := ironbark
APP_VERSION := $(shell git describe --always --dirty 2>/dev/null || echo dev)
# `BUILD_DATE` is used by the Docker `LABEL` and is in local time with offset.
BUILD_DATE := $(shell date -Isec)
# `BUILD_TIME_UTC` is what we inject into the Go binary; always UTC ISO 8601.
BUILD_TIME_UTC := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VCS_REF := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

# Linker flags used to inject build-time metadata into the binary.
# Keep in sync with the ARG/ldflags in `Dockerfile`.
GO_LDFLAGS := \
  -X brightsparklabs.com/ironbark/internal/version.Version=$(APP_VERSION) \
  -X brightsparklabs.com/ironbark/internal/version.Commit=$(VCS_REF) \
  -X brightsparklabs.com/ironbark/internal/version.BuildTime=$(BUILD_TIME_UTC)

# Files that the Go binary depends on. Built dynamically via `find` so new
# source files (Go sources, embedded resources, go.mod, go.sum) are picked
# up automatically without further Makefile edits. Anything inside the repo
# that is not a test file (`*_test.go`) is treated as a build input.
GO_SOURCES := $(shell find . \
  -type f \
  \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) \
  -not -name '*_test.go' \
  -not -path './dist/*' \
  -not -path './build/*' \
  2>/dev/null)
EMBED_SOURCES := $(shell find resources/resources -type f 2>/dev/null)
BUILD_INPUTS := $(GO_SOURCES) $(EMBED_SOURCES)

.PHONY: help
.DEFAULT: help
# The below `awk` is a simple variation for self-documenting Makefiles. See:
# https://ricardoanderegg.com/posts/makefile-python-project-tricks/
#
# Basically this allows task documentation to be appended to the task name
# with two hashes and it will be picked up in help output.
help: ## Display this help section.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z\$$/]+.*:.*?##\s/ {printf "\033[36m%-38s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: test
test: ## Run unit tests.
	go test ./...

.PHONY: format
format: ## Format the codebase.
	gofmt -w .

.PHONY: clean
clean: ## Remove the build artifacts.
	rm -rf ./build/

.PHONY: build
build: build/bin/ironbark ## Build the application.

# `build/bin/ironbark` depends on every Go source file, embedded resource,
# `go.mod` and `go.sum` (computed dynamically into `BUILD_INPUTS`). When any
# of these change Make will rebuild the binary; otherwise it is left alone.
# This avoids the "Nothing to be done" footgun without resorting to `.PHONY`.
build/bin/ironbark: $(BUILD_INPUTS) README.adoc
	mkdir -p build/bin
	# Drop the repo-root `README.adoc` into the embedded resources
	# directory so it is picked up by the `//go:embed resources/*`
	# in `resources/resources.go`. The copy is gitignored and is
	# removed after `go build` so the working tree stays clean. If
	# `go build` is invoked standalone (without `make`), the README
	# is simply absent from the embed and a fallback message is
	# returned by `ironbark docs`.
	@cp README.adoc resources/resources/README.adoc
	trap 'rm -f resources/resources/README.adoc' EXIT \
		&& go mod download \
		&& CGO_ENABLED=0 GOOS=linux go build -ldflags "$(GO_LDFLAGS)" -o build/bin/ironbark .

.PHONY: docs
docs: docs-html docs-pdf ## Generate HTML and PDF documentation from README.

.PHONY: docs-html
docs-html: build/docs/README.html ## Generate HTML documentation from README.

build/docs/README.html: README.adoc
	@mkdir -p build/docs
	asciidoctor -b html5 -o build/docs/README.html README.adoc

.PHONY: docs-pdf
docs-pdf: build/docs/README.pdf ## Generate PDF documentation from README.

build/docs/README.pdf: README.adoc
	@mkdir -p build/docs
	asciidoctor-pdf -o build/docs/README.pdf README.adoc

.PHONY: oci-image
oci-image: oci-image-k3s oci-image-rke2 oci-image-rke2-ceph ## Build K3s, RKE2, and RKE2 Ceph variant OCI images.

.PHONY: oci-image-k3s
oci-image-k3s: ## Build K3s variant OCI image (linux/amd64).
	docker buildx build \
		--target ironbark-k3s \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_TIME_UTC=$(BUILD_TIME_UTC) \
		--build-arg VCS_REF=$(VCS_REF) \
		--platform linux/amd64 \
		--load \
		-t brightsparklabs/$(APP_NAME):$(APP_VERSION) \
		-t brightsparklabs/$(APP_NAME):latest .

.PHONY: oci-image-rke2
oci-image-rke2: ## Build RKE2 variant OCI image (linux/amd64).
	docker buildx build \
		--target ironbark-rke2 \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_TIME_UTC=$(BUILD_TIME_UTC) \
		--build-arg VCS_REF=$(VCS_REF) \
		--platform linux/amd64 \
		--load \
		-t brightsparklabs/$(APP_NAME)-rke2:$(APP_VERSION) \
		-t brightsparklabs/$(APP_NAME)-rke2:latest .

.PHONY: oci-image-rke2-ceph
oci-image-rke2-ceph: ## Build RKE2 Ceph variant OCI image (linux/amd64).
	docker buildx build \
		--target ironbark-rke2-ceph \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_TIME_UTC=$(BUILD_TIME_UTC) \
		--build-arg VCS_REF=$(VCS_REF) \
		--platform linux/amd64 \
		--load \
		-t brightsparklabs/$(APP_NAME)-rke2-ceph:$(APP_VERSION) \
		-t brightsparklabs/$(APP_NAME)-rke2-ceph:latest .

.PHONY: oci-image-save
oci-image-save: oci-image-k3s-save oci-image-rke2-save oci-image-rke2-ceph-save ## Save K3s, RKE2, and RKE2 Ceph variant OCI images.

.PHONY: oci-image-k3s-save
oci-image-k3s-save: oci-image-k3s ## Save K3s variant OCI images.
	mkdir -p build/images
	docker save \
		brightsparklabs/$(APP_NAME):$(APP_VERSION) \
		-o build/images/oci-brightsparklabs-$(APP_NAME)-$(APP_VERSION).tar

.PHONY: oci-image-rke2-save
oci-image-rke2-save: oci-image-rke2 ## Save RKE2 variant OCI images.
	mkdir -p build/images
	docker save \
		brightsparklabs/$(APP_NAME)-rke2:$(APP_VERSION) \
		-o build/images/oci-brightsparklabs-$(APP_NAME)-rke2-$(APP_VERSION).tar

.PHONY: oci-image-rke2-ceph-save
oci-image-rke2-ceph-save: oci-image-rke2-ceph ## Save RKE2 Ceph variant OCI images.
	mkdir -p build/images
	docker save \
		brightsparklabs/$(APP_NAME)-rke2-ceph:$(APP_VERSION) \
		-o build/images/oci-brightsparklabs-$(APP_NAME)-rke2-ceph-$(APP_VERSION).tar


.PHONY: oci-image-push
oci-image-push: oci-image-k3s-push oci-image-rke2-push oci-image-rke2-ceph-push ## Build and push K3s, RKE2, and RKE2 Ceph images to DockerHub (linux/amd64).

.PHONY: oci-image-k3s-push
oci-image-k3s-push: ## Build and push K3s variant OCI image to DockerHub (linux/amd64).
	docker buildx build \
		--target ironbark-k3s \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_TIME_UTC=$(BUILD_TIME_UTC) \
		--build-arg VCS_REF=$(VCS_REF) \
		--platform linux/amd64 \
		--push \
		-t brightsparklabs/$(APP_NAME):$(APP_VERSION) \
		-t brightsparklabs/$(APP_NAME):latest .

.PHONY: oci-image-rke2-push
oci-image-rke2-push: ## Build and push RKE2 variant OCI image to DockerHub (linux/amd64).
	docker buildx build \
		--target ironbark-rke2 \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_TIME_UTC=$(BUILD_TIME_UTC) \
		--build-arg VCS_REF=$(VCS_REF) \
		--platform linux/amd64 \
		--push \
		-t brightsparklabs/$(APP_NAME)-rke2:$(APP_VERSION) \
		-t brightsparklabs/$(APP_NAME)-rke2:latest .

.PHONY: oci-image-rke2-ceph-push
oci-image-rke2-ceph-push: ## Build and push RKE2 Ceph variant OCI image to DockerHub (linux/amd64).
	docker buildx build \
		--target ironbark-rke2-ceph \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_TIME_UTC=$(BUILD_TIME_UTC) \
		--build-arg VCS_REF=$(VCS_REF) \
		--platform linux/amd64 \
		--push \
		-t brightsparklabs/$(APP_NAME)-rke2-ceph:$(APP_VERSION) \
		-t brightsparklabs/$(APP_NAME)-rke2-ceph:latest .

.PHONY: dist
dist: oci-image-save docs ## Create distribution with images and documentation.
	@echo "$$(date -Isec) Creating distribution in build/dist/"
	@rm -rf build/dist
	@mkdir -p build/dist

	@# Hardlink image tarballs to save space.
	@for img in build/images/*.tar; do \
		if [ -f "$$img" ]; then \
			ln "$$img" "build/dist/$$(basename $$img)"; \
		fi; \
	done

	@# Copy documentation.
	@ln build/docs/README.html build/dist/README.html
	@ln build/docs/README.pdf build/dist/README.pdf

	@# Generate checksums.
	@cd build/dist && for f in *; do \
		echo "$$(date -Isec) Generating checksum for $$f ..."; \
		sha256sum "$$f" > "$$f.sha256"; \
	done

	@echo ""
	@echo "Total size:"
	@du -sh build/dist/

.PHONY: dist-info
dist-info: ## Prints out details of the distribution.
	@echo ""
	@echo Ironbark release: $(APP_VERSION)
	@echo ""
	@echo Contents:
	@ls -1 build/dist | sed 's/^/  - /'
	@echo ""
	@echo "SHA-256 Checksums:"
	@cd build/dist && (cat *.sha256 | sed 's/^/  /')

.PHONY: test-coverage
test-coverage: ## Run unit tests with coverage reporting.
	go test -v -race -coverprofile=coverage.out ./... \
		&& go tool cover -func=coverage.out \
		&& echo "Coverage summary:" \
		&& go tool cover -func=coverage.out | tail -1

.PHONY: check-format
check-format: ## Check if code is formatted correctly (fails if not).
	if [ "$$(gofmt -l . | wc -l)" -gt 0 ]; then \
		echo "The following files need formatting:"; \
		gofmt -l .; \
		exit 1; \
	fi

.PHONY: check-vuln
check-vuln: ## Run vulnerability scanner (govulncheck).
	go install golang.org/x/vuln/cmd/govulncheck@latest \
		&& govulncheck ./...

.PHONY: check-release
check-release: ## Validate GoReleaser configuration.
	goreleaser check

.PHONY: check
check: check-format check-vuln check-release ## Run all checks (format, vulnerabilities, release config).

# ------------------------------------------------------------------------------
# GoReleaser Targets
# ------------------------------------------------------------------------------

.PHONY: release-snapshot
release-snapshot: ## Build a snapshot release locally (all platforms, no publish).
	GOMAXPROCS=$(GOMAXPROCS) goreleaser release --snapshot --clean

.PHONY: release-build
release-build: ## Build binaries for all platforms (no archives).
	GOMAXPROCS=$(GOMAXPROCS) goreleaser build --snapshot --clean

# ------------------------------------------------------------------------------
# Development Setup
# ------------------------------------------------------------------------------

.PHONY: setup-hooks
setup-hooks: ## Install and configure prek git hooks.
	@command -v prek >/dev/null 2>&1 || { \
		echo "Installing prek..."; \
		go install github.com/j178/prek@latest; \
	}
	prek install
	@echo "Git hooks configured with prek. Pre-commit will auto-format Go code."
