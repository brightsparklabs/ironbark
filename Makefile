##
 # Makefile
 # ______________________________________________________________________________
 #
 # Created by brightSPARK Labs
 # www.brightsparklabs.com
 ##

# Warn whenever make sees a reference to an undefined variable.
MAKEFLAGS += --warn-undefined-variables
# Disable implicit rules as they were not designed for python.
MAKEFLAGS += --no-builtin-rules
# Set the shell to `bash` to give better error messaging/handling.
# See https://stackoverflow.com/questions/20615217/bash-bad-substitution
SHELL := bash

APP_NAME=ironbark
APP_VERSION=$(shell git describe --always --dirty 2>/dev/null || echo dev)
# `BUILD_DATE` is used by the Docker `LABEL` and is in local time with offset.
BUILD_DATE=$(shell date -Isec)
# `BUILD_TIME_UTC` is what we inject into the Go binary; always UTC ISO 8601.
BUILD_TIME_UTC=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VCS_REF=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

# Linker flags used to inject build-time metadata into the binary.
# Keep in sync with the ARG/ldflags in `Dockerfile`.
GO_LDFLAGS= \
  -X brightsparklabs.com/ironbark/internal/version.Version=$(APP_VERSION) \
  -X brightsparklabs.com/ironbark/internal/version.Commit=$(VCS_REF) \
  -X brightsparklabs.com/ironbark/internal/version.BuildTime=$(BUILD_TIME_UTC)
# Format and linter rules to ignore.
# See https://docs.astral.sh/ruff/rules/
# Ignore lambda functions.
RULES=E731

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
build/bin/ironbark:
	mkdir -p build/bin
	cd src \
		&& go mod download \
		&& CGO_ENABLED=0 GOOS=linux go build -ldflags "$(GO_LDFLAGS)" -o ../build/bin/ironbark .

.PHONY: oci-image
oci-image: ## Build OCI images.
	docker build \
		--build-arg APP_VERSION=${APP_VERSION} \
		--build-arg BUILD_DATE=${BUILD_DATE} \
		--build-arg BUILD_TIME_UTC=${BUILD_TIME_UTC} \
		--build-arg VCS_REF=${VCS_REF} \
		-t brightsparklabs/${APP_NAME}:${APP_VERSION} \
		-t brightsparklabs/${APP_NAME}:latest .

.PHONY: oci-image-save
oci-image-save: oci-image ## Save OCI images.
	mkdir -p build/images
	docker save \
		brightsparklabs/${APP_NAME}:${APP_VERSION} \
		-o build/images/oci-brightsparklabs-${APP_NAME}-${APP_VERSION}.tar
