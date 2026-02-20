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
APP_VERSION=$(shell git describe --always --dirty)
# As this is a python project, we want this to be PEP440 compliant.
APP_VERSION_PYTHON=$(shell echo "${APP_VERSION}" | sed -E 's/^v//; s/-([0-9]+)-g([0-9a-f]+)/.\1+\2/; s/-dirty/.dirty/')
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
build: ## Remove the build artifacts.
	mkdir -p build/bin
	cd src \
		&& go mod download \
		&& CGO_ENABLED=0 GOOS=linux go build -o ../build/bin/ironbark .

.PHONY: docker
docker: ## Build Docker images.
	docker build \
		-t docker.brightsparklabs.com/brightsparklabs/${APP_NAME}:${APP_VERSION} \
		-t docker.brightsparklabs.com/brightsparklabs/${APP_NAME}:latest .

.PHONY: docker-save
docker-save: docker ## Save Docker images.
	mkdir -p build/images
	docker save \
		docker.brightsparklabs.com/brightsparklabs/${APP_NAME}:${APP_VERSION} \
		-o build/images/oci-docker.brightsparklabs.com-${APP_NAME}-${APP_VERSION}.tar
