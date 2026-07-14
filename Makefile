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
# up automatically without further Makefile edits. Anything inside `src/`
# that is not a test file (`*_test.go`) is treated as a build input.
GO_SOURCES := $(shell find src \
  -type f \
  \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) \
  -not -name '*_test.go' \
  2>/dev/null)
EMBED_SOURCES := $(shell find src/resources/resources -type f 2>/dev/null)
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
	cd src \
		&& go test ./...

.PHONY: format
format: ## Format the codebase.
	cd src \
		&& gofmt -w .

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
	# in `src/resources/resources.go`. The copy is gitignored and is
	# removed after `go build` so the working tree stays clean. If
	# `go build` is invoked standalone (without `make`), the README
	# is simply absent from the embed and a fallback message is
	# returned by `ironbark docs`.
	@cp README.adoc src/resources/resources/README.adoc
	cd src \
		&& trap 'rm -f resources/resources/README.adoc' EXIT \
		&& go mod download \
		&& CGO_ENABLED=0 GOOS=linux go build -ldflags "$(GO_LDFLAGS)" -o ../build/bin/ironbark .

.PHONY: oci-image
oci-image: oci-image-k3s oci-image-rke2 ## Build both K3s and RKE2 variant OCI images.

.PHONY: oci-image-k3s
oci-image-k3s: ## Build K3s variant OCI image.
	docker build \
		--target ironbark-k3s \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_TIME_UTC=$(BUILD_TIME_UTC) \
		--build-arg VCS_REF=$(VCS_REF) \
		-t brightsparklabs/$(APP_NAME):$(APP_VERSION) \
		-t brightsparklabs/$(APP_NAME):latest .

.PHONY: oci-image-rke2
oci-image-rke2: ## Build RKE2 variant OCI image.
	docker build \
		--target ironbark-rke2 \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_TIME_UTC=$(BUILD_TIME_UTC) \
		--build-arg VCS_REF=$(VCS_REF) \
		-t brightsparklabs/$(APP_NAME)-rke2:$(APP_VERSION) \
		-t brightsparklabs/$(APP_NAME)-rke2:latest .

.PHONY: oci-image-save
oci-image-save: oci-image-k3s-save oci-image-rke2-save ## Save both K3s and RKE2 variant OCI images.

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

# ------------------------------------------------------------------------------
# Multi-arch OCI Image Targets (using buildx)
# ------------------------------------------------------------------------------
# These targets use Docker buildx to build multi-architecture images.
# Buildx is required for publishing to container registries with multi-arch
# support (linux/amd64 and linux/arm64).
#
# Why buildx instead of Podman:
# - GitHub Actions runners use Docker by default
# - Buildx provides native multi-arch build support
# - Consistent with existing CI/CD patterns
# - Podman is still preferred for local development (see devbox.json)

.PHONY: oci-image-buildx
oci-image-buildx: oci-image-k3s-buildx oci-image-rke2-buildx ## Build both K3s and RKE2 multi-arch OCI images.

.PHONY: oci-image-k3s-buildx
oci-image-k3s-buildx: ## Build K3s variant multi-arch OCI image (linux/amd64,linux/arm64).
	docker buildx build \
		--target ironbark-k3s \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_TIME_UTC=$(BUILD_TIME_UTC) \
		--build-arg VCS_REF=$(VCS_REF) \
		--platform linux/amd64,linux/arm64 \
		--load \
		-t brightsparklabs/$(APP_NAME):$(APP_VERSION) \
		-t brightsparklabs/$(APP_NAME):latest .

.PHONY: oci-image-rke2-buildx
oci-image-rke2-buildx: ## Build RKE2 variant multi-arch OCI image (linux/amd64,linux/arm64).
	docker buildx build \
		--target ironbark-rke2 \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_TIME_UTC=$(BUILD_TIME_UTC) \
		--build-arg VCS_REF=$(VCS_REF) \
		--platform linux/amd64,linux/arm64 \
		--load \
		-t brightsparklabs/$(APP_NAME)-rke2:$(APP_VERSION) \
		-t brightsparklabs/$(APP_NAME)-rke2:latest .

.PHONY: oci-image-push
oci-image-push: oci-image-k3s-push oci-image-rke2-push ## Build and push both K3s and RKE2 multi-arch images to DockerHub.

.PHONY: oci-image-k3s-push
oci-image-k3s-push: ## Build and push K3s variant multi-arch OCI image to DockerHub.
	docker buildx build \
		--target ironbark-k3s \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_TIME_UTC=$(BUILD_TIME_UTC) \
		--build-arg VCS_REF=$(VCS_REF) \
		--platform linux/amd64,linux/arm64 \
		--push \
		-t brightsparklabs/$(APP_NAME):$(APP_VERSION) \
		-t brightsparklabs/$(APP_NAME):latest .

.PHONY: oci-image-rke2-push
oci-image-rke2-push: ## Build and push RKE2 variant multi-arch OCI image to DockerHub.
	docker buildx build \
		--target ironbark-rke2 \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg BUILD_TIME_UTC=$(BUILD_TIME_UTC) \
		--build-arg VCS_REF=$(VCS_REF) \
		--platform linux/amd64,linux/arm64 \
		--push \
		-t brightsparklabs/$(APP_NAME)-rke2:$(APP_VERSION) \
		-t brightsparklabs/$(APP_NAME)-rke2:latest .
