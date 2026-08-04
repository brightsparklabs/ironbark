##
 # The image used for k8s deployment.
 # _____________________________________________________________________________
 #
 # Created by brightSPARK Labs
 # www.brightsparklabs.com
##

# ------------------------------------------------------------------------------
# CONSTANTS
# ------------------------------------------------------------------------------

# `TARGETARCH` is automatically set by BuildKit (e.g. `amd64`, `arm64`) based
# on the target platform (`--platform`). Default to it so cross-builds work
# without an explicit `--build-arg ARCH=...`, but allow overriding for
# legacy invocations.
ARG TARGETARCH
ARG ARCH=${TARGETARCH:-amd64}
ARG UBUNTU_IMAGE=ubuntu:24.04
ARG ALPINE_IMAGE=alpine:3.24
ARG GOLANG_VERSION=1.26.5

# Tool versions.
ARG KUBECTL_VERSION=v1.36.1
ARG ZARF_VERSION=v0.76.0
ARG RKE2_VERSION=v1.33.5+rke2r1
ARG LOCAL_PATH_PROVISIONER_VERSION=v0.0.30
ARG ROOK_VERSION=v1.15.8
ARG CEPH_VERSION=v18.2.4
ARG CEPHCSI_VERSION=v3.12.2

# ------------------------------------------------------------------------------
# BUILDER STAGE - TOOLING
# ------------------------------------------------------------------------------

FROM ${UBUNTU_IMAGE} AS builder-tooling

ARG ARCH
ARG KUBECTL_VERSION
ARG ZARF_VERSION

# Use bash (not the default `/bin/sh` → `dash` on Debian/Ubuntu) with
# strict error handling for every `RUN` in this stage. This is the
# Docker-recommended way to enable `set -euo pipefail` semantics
# globally, and avoids the `dash: Illegal option -o pipefail` error
# that occurs if `set -o pipefail` is used in a default `RUN`.
SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

# `ca-certificates` is required for HTTPS downloads from `dl.k8s.io` and
# `github.com`. `curl` is preferred over `ADD <url>` because `ADD <url>` is
# discouraged by Docker's best-practices guide (no retry, no checksum, no
# extraction, extra layer overhead).
RUN apt-get update \
      && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl

WORKDIR /build/bin
# Download `kubectl` for the target architecture. The `--retry`/`--fail`
# flags give us proper error handling that `ADD <url>` does not.
RUN curl --fail --silent --show-error --location --retry 3 \
      --output kubectl \
      "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl" \
      && chmod +x kubectl

RUN curl --fail --silent --show-error --location --retry 3 \
      --output zarf \
      "https://github.com/zarf-dev/zarf/releases/download/${ZARF_VERSION}/zarf_${ZARF_VERSION}_Linux_${ARCH}" \
      && chmod +x zarf

# Make a self-contained directory which can be used to do `zarf init`.
# This allows a single directory to be copied onto host if installing k3s.
#
# IMPORTANT: this directory (under `/app/resources/zarf/init` in the final
# image after the `COPY --from=builder-tooling /build/ .` below) and the
# layout of its contents (the `zarf` binary plus a `zarf-init-*.tar.zst`
# package) are a contract with the Go code that ships the
# `generate zarf-bootstrap` command. Any change to this path or the
# expected filenames MUST be mirrored in:
#   - `cmd/generate.go`
#       - `defaultZarfBootstrapSourcePath`
#       - `zarfBinaryName`
#       - `zarfInitPackagePrefix` / `zarfInitPackageSuffix`
#   - `resources/resources/zarf-bootstrap.sh.tmpl` (which references the
#     same defaults via the rendered template).
# Otherwise the asset-existence check in `execZarfBootstrap` will fail at
# runtime even though the assets are present in the image.
WORKDIR /build/resources/zarf/init
# Hard link to save space.
RUN ln /build/bin/zarf
# URL from: https://docs.zarf.dev/best-practices/upgrading-zarf/
RUN curl --fail --silent --show-error --location --retry 3 \
      --output "zarf-init-${ARCH}-${ZARF_VERSION}.tar.zst" \
      "https://github.com/zarf-dev/zarf/releases/download/${ZARF_VERSION}/zarf-init-${ARCH}-${ZARF_VERSION}.tar.zst"

# Build each package. The stage-level `SHELL` directive above runs every
# `RUN` under `bash -euo pipefail`, so any package-create failure aborts
# the build rather than silently producing a partial image.
WORKDIR /src/zarf/packages
COPY zarf-packages/ .
RUN for package_type in *; do \
      for package_dir in "${package_type}"/*; do \
        /build/bin/zarf package create "${package_dir}" -o "/build/resources/packages/${package_type}/"; \
      done; \
    done

# ------------------------------------------------------------------------------
# BUILDER STAGE - RKE2 ARTIFACTS (OPTIONAL)
# ------------------------------------------------------------------------------

# This stage downloads RKE2 artifacts for air-gapped deployment. It is only
# included in the final image when building the `ironbark-rke2` target.
# The default `ironbark` target (K3s via Zarf) does not include these files.
FROM ${UBUNTU_IMAGE} AS builder-rke2-artifacts

ARG ARCH
ARG RKE2_VERSION
ARG LOCAL_PATH_PROVISIONER_VERSION

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN apt-get update \
      && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl

# Make a self-contained directory which can be used to install RKE2.
# This allows a single directory to be copied onto host if installing RKE2.
#
# IMPORTANT: this directory (under `/app/resources/rke2/` in the final
# image after the `COPY --from=builder-rke2-artifacts /build/ .` below) and
# the layout of its contents are a contract with the Go code that ships the
# `generate rke2-bootstrap` command. Any change to this path or the
# expected filenames MUST be mirrored in:
#   - `cmd/generate.go`
#       - `defaultRke2BootstrapSourcePath`
#       - RKE2 artifact filename constants
#   - `resources/resources/rke2-bootstrap.sh.tmpl` (which references the
#     same defaults via the rendered template).
# Otherwise the asset-existence check in `execRke2Bootstrap` will fail at
# runtime even though the assets are present in the image.
WORKDIR /build/resources/rke2

# Download RKE2 installation script.
RUN curl --fail --silent --show-error --location --retry 3 \
      --output install.sh \
      "https://get.rke2.io"

# Download RKE2 binary tarball.
RUN curl --fail --silent --show-error --location --retry 3 \
      --output "rke2.linux-${ARCH}.tar.gz" \
      "https://github.com/rancher/rke2/releases/download/${RKE2_VERSION}/rke2.linux-${ARCH}.tar.gz"

# Download RKE2 images (using Cilium CNI). This is the largest artifact (~2GB).
RUN curl --fail --silent --show-error --location --retry 3 \
      --output "rke2-images-cilium.linux-${ARCH}.tar.zst" \
      "https://github.com/rancher/rke2/releases/download/${RKE2_VERSION}/rke2-images-cilium.linux-${ARCH}.tar.zst"

# Download RKE2 core image tarball (contains runtime and essential images).
RUN curl --fail --silent --show-error --location --retry 3 \
      --output "rke2-images-core.linux-${ARCH}.tar.zst" \
      "https://github.com/rancher/rke2/releases/download/${RKE2_VERSION}/rke2-images-core.linux-${ARCH}.tar.zst"

# Download checksums for verification.
RUN curl --fail --silent --show-error --location --retry 3 \
      --output "sha256sum-${ARCH}.txt" \
      "https://github.com/rancher/rke2/releases/download/${RKE2_VERSION}/sha256sum-${ARCH}.txt"

# Verify checksums of downloaded artifacts.
RUN sha256sum -c --ignore-missing "sha256sum-${ARCH}.txt"

# Install skopeo and zstd for pulling and compressing the local-path-provisioner image.
RUN apt-get update \
      && apt-get install -y --no-install-recommends \
        skopeo \
        zstd

# Download local-path-provisioner image for CSI driver.
# This is saved and loaded during RKE2 bootstrap to provide persistent storage.
# We use skopeo to pull the image without needing a Docker daemon.
RUN skopeo copy \
      docker://rancher/local-path-provisioner:${LOCAL_PATH_PROVISIONER_VERSION} \
      docker-archive:/tmp/local-path-provisioner.tar:rancher/local-path-provisioner:${LOCAL_PATH_PROVISIONER_VERSION} \
      && zstd -T0 -19 /tmp/local-path-provisioner.tar \
      -o "rke2-images-local-path.linux-${ARCH}.tar.zst" \
      && rm /tmp/local-path-provisioner.tar

RUN cat > VERSION.json <<EOF
{
  "ironbark": "rke2-variant",
  "tools": {
    "rke2": "${RKE2_VERSION}",
    "local-path-provisioner": "${LOCAL_PATH_PROVISIONER_VERSION}"
  }
}
EOF

# Copy the RKE2 configuration template.
COPY resources/resources/rke2-config.yaml.tmpl config.yaml.template

# Copy the Cilium configuration for kube-proxy replacement.
# This configures Cilium to use localhost for API access, avoiding firewall issues.
COPY resources/resources/rke2-cilium-config.yaml.tmpl rke2-cilium-config.yaml

# Copy the CSI manifest so it's included in the extracted RKE2 artifacts.
COPY resources/resources/csi-local-path-provisioner.yaml.tmpl csi-local-path-provisioner.yaml

# ------------------------------------------------------------------------------
# BUILDER STAGE - RKE2 CEPH ARTIFACTS
# ------------------------------------------------------------------------------

# Build RKE2 Ceph variant artifacts by inheriting from the standard RKE2 stage
# and adding Rook-Ceph container images.
FROM builder-rke2-artifacts AS builder-rke2-ceph-artifacts

ARG ARCH
ARG ROOK_VERSION
ARG CEPH_VERSION
ARG CEPHCSI_VERSION

# Enable strict error handling to catch failures immediately during image downloads.
# Without this, failed downloads or pipeline errors could silently produce incomplete
# artifacts, causing hard-to-debug runtime failures in deployed clusters.
SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

# Install Helm for downloading the Rook Helm chart.
RUN curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash \
      && helm repo add rook-release https://charts.rook.io/release \
      && helm repo update

# Download all Rook-Ceph images as separate OCI tarballs.
# RKE2 expects OCI layout format (manifest.json + blobs/), not docker-archive format.
# Each image gets its own OCI directory that is then tarred separately.

# Download Rook operator image.
RUN skopeo copy \
      docker://rook/ceph:${ROOK_VERSION} \
      oci:/tmp/rook-ceph \
      && tar -I 'zstd -19 -T0' -cf rke2-images-rook-ceph.linux-amd64.tar.zst -C /tmp/rook-ceph . \
      && rm -rf /tmp/rook-ceph

# Download Ceph cluster image.
RUN skopeo copy \
      docker://quay.io/ceph/ceph:${CEPH_VERSION} \
      oci:/tmp/ceph \
      && tar -I 'zstd -19 -T0' -cf rke2-images-ceph.linux-amd64.tar.zst -C /tmp/ceph . \
      && rm -rf /tmp/ceph

# Download Ceph CSI driver image.
RUN skopeo copy \
      docker://quay.io/cephcsi/cephcsi:${CEPHCSI_VERSION} \
      oci:/tmp/cephcsi \
      && tar -I 'zstd -19 -T0' -cf rke2-images-cephcsi.linux-amd64.tar.zst -C /tmp/cephcsi . \
      && rm -rf /tmp/cephcsi

# Download CSI sidecar images (using latest stable versions).
# These are required for CSI driver operation.
RUN skopeo copy \
      docker://registry.k8s.io/sig-storage/csi-provisioner:v5.2.0 \
      oci:/tmp/csi-provisioner \
      && tar -I 'zstd -19 -T0' -cf rke2-images-csi-provisioner.linux-amd64.tar.zst -C /tmp/csi-provisioner . \
      && rm -rf /tmp/csi-provisioner

RUN skopeo copy \
      docker://registry.k8s.io/sig-storage/csi-attacher:v4.8.1 \
      oci:/tmp/csi-attacher \
      && tar -I 'zstd -19 -T0' -cf rke2-images-csi-attacher.linux-amd64.tar.zst -C /tmp/csi-attacher . \
      && rm -rf /tmp/csi-attacher

RUN skopeo copy \
      docker://registry.k8s.io/sig-storage/csi-resizer:v1.13.2 \
      oci:/tmp/csi-resizer \
      && tar -I 'zstd -19 -T0' -cf rke2-images-csi-resizer.linux-amd64.tar.zst -C /tmp/csi-resizer . \
      && rm -rf /tmp/csi-resizer

RUN skopeo copy \
      docker://registry.k8s.io/sig-storage/csi-snapshotter:v8.2.1 \
      oci:/tmp/csi-snapshotter \
      && tar -I 'zstd -19 -T0' -cf rke2-images-csi-snapshotter.linux-amd64.tar.zst -C /tmp/csi-snapshotter . \
      && rm -rf /tmp/csi-snapshotter

RUN skopeo copy \
      docker://registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.13.0 \
      oci:/tmp/csi-node-driver-registrar \
      && tar -I 'zstd -19 -T0' -cf rke2-images-csi-node-driver-registrar.linux-amd64.tar.zst -C /tmp/csi-node-driver-registrar . \
      && rm -rf /tmp/csi-node-driver-registrar

# Download Rook Helm chart for air-gapped deployment.
# Base64-encode it and inject into the HelmChart manifest as chartContent.
# This is required because RKE2's helm-install pod cannot access host filesystem paths.
RUN helm fetch rook-release/rook-ceph \
      --version ${ROOK_VERSION} \
      --destination /tmp

# Copy the HelmChart manifest template and inject the base64-encoded chart.
# The __CHART_CONTENT_BASE64__ placeholder will be replaced with the actual chart content.
COPY resources/resources/csi-rook-ceph-chart.yaml.tmpl /tmp/csi-rook-ceph-chart.yaml.tmpl
RUN CHART_CONTENT=$(base64 -w 0 /tmp/rook-ceph-${ROOK_VERSION}.tgz) \
      && sed "s|__CHART_CONTENT_BASE64__|${CHART_CONTENT}|" \
            /tmp/csi-rook-ceph-chart.yaml.tmpl > csi-rook-ceph-chart.yaml \
      && rm /tmp/csi-rook-ceph-chart.yaml.tmpl /tmp/rook-ceph-${ROOK_VERSION}.tgz

# Remove the local-path provisioner image tarball as we're using Ceph instead.
RUN rm -f rke2-images-local-path.linux-*.tar.zst

# Copy CephCluster CR (this is applied after the operator is deployed).
# Order matters (alphabetical):
#   1. csi-rook-ceph-chart.yaml - HelmChart CRD deploys operator/CRDs/RBAC (with embedded chart)
#   2. csi-rook-ceph-cluster.yaml - CephCluster/Pool/StorageClass
COPY resources/resources/csi-rook-ceph-cluster.yaml.tmpl csi-rook-ceph-cluster.yaml

# Remove the local-path CSI manifest.
RUN rm -f csi-local-path-provisioner.yaml

# Update VERSION.json to reflect Ceph variant.
RUN cat > VERSION.json <<EOF
{
  "ironbark": "rke2-ceph-variant",
  "tools": {
    "rke2": "${RKE2_VERSION}",
    "rook": "${ROOK_VERSION}",
    "ceph": "${CEPH_VERSION}",
    "cephcsi": "${CEPHCSI_VERSION}"
  }
}
EOF

# ------------------------------------------------------------------------------
# BUILDER STAGE - GOLANG
# ------------------------------------------------------------------------------

FROM golang:${GOLANG_VERSION} AS builder-golang
ARG APP_VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_TIME_UTC=unknown
ARG BUILD_DATE=unknown

# Use bash with strict error handling for every `RUN` in this stage —
# same rationale as `builder-tooling`. Defensive: nothing in this stage
# currently uses pipes, but this guards against future additions
# silently swallowing errors due to `dash` being the default `sh`.
SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

# The Go binary is built via the repo-root `Makefile` (rather than calling
# `go build` directly) so there is a single canonical build path shared by
# local development and the container image. This guarantees that any
# Makefile-only build steps (e.g. embedding `README.adoc` into the binary
# for the `ironbark docs` command) are exercised here as well.
RUN apt-get update \
      && apt-get install -y --no-install-recommends make

WORKDIR /build

# Pre-fetch Go module dependencies in their own layer so they are cached
# independently of the rest of the source tree.
COPY go.mod go.sum ./
RUN go mod download

# Copy the remaining build inputs the Makefile expects. The container
# build context intentionally excludes `.git`, so the Makefile's
# `git describe` / `git rev-parse` invocations will fall back to their
# `|| echo ...` defaults. We therefore pass the real build metadata
# (resolved from `git` on the host by the Makefile's `oci-image` target)
# in as build args and forward them to `make` as variable overrides —
# command-line assignments take precedence over the Makefile's `:=`
# assignments, so the ldflags receive the correct values.
COPY Makefile README.adoc ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY resources/ ./resources/
COPY main.go ./

RUN make build \
      APP_VERSION=${APP_VERSION} \
      VCS_REF=${VCS_REF} \
      BUILD_TIME_UTC=${BUILD_TIME_UTC} \
      BUILD_DATE=${BUILD_DATE}

# ------------------------------------------------------------------------------
# FINAL STAGE - BASE (SHARED)
# ------------------------------------------------------------------------------

# Common base for both K3s and RKE2 variants. This stage contains everything
# except the K8s distribution artifacts (Zarf init or RKE2 files).
FROM ${ALPINE_IMAGE} AS ironbark-base

ARG IRONBARK_DATA_DIR=/mnt/data
ARG IRONBARK_INTERNAL_PACKAGES_DIR=/app/resources/packages
ARG APP_VERSION=dev
ARG BUILD_DATE
ARG VCS_REF
ARG KUBECTL_VERSION
ARG ZARF_VERSION
ARG RKE2_VERSION
ARG LOCAL_PATH_PROVISIONER_VERSION

# Create comprehensive VERSION.json file with all tool versions.
# This file is included in all Ironbark image variants for version tracking.
RUN mkdir -p /app && cat > /app/VERSION.json <<EOF
{
  "ironbark": "${APP_VERSION}",
  "build_date": "${BUILD_DATE}",
  "vcs_ref": "${VCS_REF}",
  "tools": {
    "kubectl": "${KUBECTL_VERSION}",
    "zarf": "${ZARF_VERSION}",
    "rke2": "${RKE2_VERSION}",
    "local-path-provisioner": "${LOCAL_PATH_PROVISIONER_VERSION}"
  }
}
EOF

# `IRONBARK_IN_CONTAINER` is baked into the image so any process started from
# this image (whether via the launcher script or an ad-hoc `podman run`) can
# unambiguously detect that it is executing inside the Ironbark container.
#
# `PATH` is set explicitly (rather than prepending to an inherited `${PATH}`)
# because the `scratch` base provides no parent environment. Note that
# `PATH` is still required even though `scratch` ships no shell — it is
# consumed by Go's `os/exec.LookPath` (used throughout the `ironbark`
# binary to locate the bundled `kubectl` and `zarf` binaries under
# `/app/bin`, and by `ironbark exec` to discover any bundled tools by
# name rather than absolute path). The conventional Linux defaults are
# listed after `/app/bin` to keep behaviour predictable if the image is
# ever rebased on a distro that ships those directories.
ENV \
  IRONBARK_DATA_DIR=${IRONBARK_DATA_DIR} \
  IRONBARK_INTERNAL_PACKAGES_DIR=${IRONBARK_INTERNAL_PACKAGES_DIR} \
  IRONBARK_IN_CONTAINER=true \
  PATH="/app/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
  META_BUILD_DATE=${BUILD_DATE} \
  META_VCS_REF=${VCS_REF} \
  APP_VERSION=${APP_VERSION} \
  HOME=/tmp

# Pre-create `/tmp` so it exists as an empty directory in the final
# image. The `scratch` base contains no filesystem layout at all, and
# any process that looks up `$HOME` (or otherwise expects `/tmp` to
# already exist) will fail on a missing path without this. The
# adjacent `ENV HOME=/tmp` then ensures the running `ironbark` binary
# - and any `zarf` / `kubectl` subprocesses it spawns - have a valid,
# writable home directory even though `scratch` ships no `/etc/passwd`
# entry to resolve via `os/user`.
WORKDIR /tmp
WORKDIR /app
COPY --from=builder-tooling /build/ .
COPY --from=builder-golang /build/build/bin/ironbark bin/

# Default to running in serve mode (API server).
# Users can override by specifying a command: docker run ... ironbark init all
CMD ["serve"]

ENTRYPOINT ["/app/bin/ironbark"]

# Expose API server port (8080) and Git proxy port (3000).
# These are used when running Ironbark in serve mode.
EXPOSE 8080 3000

# ------------------------------------------------------------------------------
# FINAL STAGE - K3S VARIANT (DEFAULT)
# ------------------------------------------------------------------------------

# Default target: K3s via Zarf init. This is the original behaviour and
# remains the default when no target is specified.
FROM ironbark-base AS ironbark-k3s

LABEL org.label-schema.name="ironbark" \
      org.label-schema.description="Kubernetes management using the brightSPARK Labs opinionated deployment pattern (K3s via Zarf)" \
      org.opencontainers.image.authors="brightSPARK Labs <enquire@brightsparklabs.com>" \
      org.label-schema.vendor="brightSPARK Labs" \
      org.label-schema.schema-version="1.0.0-rc1" \
      org.label-schema.vcs-url="https://github.com/brightsparklabs/ironbark" \
      org.label-schema.vcs-ref=${VCS_REF} \
      org.label-schema.build-date=${BUILD_DATE}

# ------------------------------------------------------------------------------
# FINAL STAGE - RKE2 VARIANT
# ------------------------------------------------------------------------------

# RKE2 target: includes RKE2 artifacts for air-gapped deployment.
# Build with: docker build --target ironbark-rke2 ...
FROM ironbark-base AS ironbark-rke2

# Set environment variable to indicate RKE2 variant.
ENV IRONBARK_RKE2_AVAILABLE=true

# Copy RKE2 artifacts from the RKE2 builder stage to /app/resources/rke2.
# This path is consistent with other resources and used by the artifact download API.
COPY --from=builder-rke2-artifacts /build/resources/rke2/ resources/rke2/

LABEL org.label-schema.name="ironbark-rke2" \
      org.label-schema.description="Kubernetes management using the brightSPARK Labs opinionated deployment pattern (RKE2)" \
      org.opencontainers.image.authors="brightSPARK Labs <enquire@brightsparklabs.com>" \
      org.label-schema.vendor="brightSPARK Labs" \
      org.label-schema.schema-version="1.0.0-rc1" \
      org.label-schema.vcs-url="https://github.com/brightsparklabs/ironbark" \
      org.label-schema.vcs-ref=${VCS_REF} \
      org.label-schema.build-date=${BUILD_DATE}

# ------------------------------------------------------------------------------
# FINAL STAGE - RKE2 CEPH VARIANT
# ------------------------------------------------------------------------------

# RKE2 Ceph target: includes RKE2 artifacts with Rook-Ceph for distributed storage.
# Build with: docker build --target ironbark-rke2-ceph ...
FROM ironbark-base AS ironbark-rke2-ceph

# Set environment variables to indicate RKE2 Ceph variant.
ENV IRONBARK_RKE2_AVAILABLE=true \
    IRONBARK_RKE2_VARIANT=ceph

# Copy RKE2 Ceph artifacts from the RKE2 Ceph builder stage to /app/resources/rke2.
# This path is consistent with other resources and used by the artifact download API.
COPY --from=builder-rke2-ceph-artifacts /build/resources/rke2/ resources/rke2/

LABEL org.label-schema.name="ironbark-rke2-ceph" \
      org.label-schema.description="Kubernetes management using the brightSPARK Labs opinionated deployment pattern (RKE2 with Rook-Ceph storage)" \
      org.opencontainers.image.authors="brightSPARK Labs <enquire@brightsparklabs.com>" \
      org.label-schema.vendor="brightSPARK Labs" \
      org.label-schema.schema-version="1.0.0-rc1" \
      org.label-schema.vcs-url="https://github.com/brightsparklabs/ironbark" \
      org.label-schema.vcs-ref=${VCS_REF} \
      org.label-schema.build-date=${BUILD_DATE}
