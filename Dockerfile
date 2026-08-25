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
ARG KUBECTL_VERSION=v1.36.3
ARG ZARF_VERSION=v0.76.0
ARG RKE2_VERSION=v1.36.3+rke2r1
ARG LOCAL_PATH_PROVISIONER_VERSION=v0.0.30
ARG ROOK_VERSION=v1.20.6
ARG CEPH_VERSION=v20.2.4

# CSI image versions - MUST match Rook Helm chart defaults for ROOK_VERSION.
# To update: Check https://github.com/rook/rook/blob/v1.20.6/deploy/charts/rook-ceph/values.yaml
# and update these ARGs to match the default image tags in the chart.
ARG CEPHCSI_VERSION=v3.17.0
ARG CSI_PROVISIONER_VERSION=v6.2.0
ARG CSI_ATTACHER_VERSION=v4.12.0
ARG CSI_RESIZER_VERSION=v2.1.0
ARG CSI_SNAPSHOTTER_VERSION=v8.5.0
ARG CSI_NODE_DRIVER_REGISTRAR_VERSION=v2.17.0
ARG CSI_ADDONS_VERSION=v0.14.0

# Ceph CSI Operator version - MUST match the subchart version in Rook Helm chart.
# To update: Check https://github.com/rook/rook/blob/v1.20.6/deploy/charts/rook-ceph/Chart.yaml
# for the ceph-csi-operator dependency version.
ARG CEPH_CSI_OPERATOR_VERSION=v1.0.4

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

# Install skopeo and zstd for pulling and compressing images.
RUN apt-get update \
      && apt-get install -y --no-install-recommends \
        skopeo \
        zstd

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
COPY resources/resources/rke2/rke2-config.yaml .

# Copy the Cilium configuration for kube-proxy replacement.
# This configures Cilium to use localhost for API access, avoiding firewall issues.
COPY resources/resources/rke2/rke2-cilium-config.yaml .

# Copy the CSI manifest so it's included in the extracted RKE2 artifacts.
COPY resources/resources/rke2/csi-local-path-provisioner.yaml csi/manifests/.

# ------------------------------------------------------------------------------
# BUILDER STAGES - CONTAINER IMAGE DOWNLOADS (PARALLEL)
# ------------------------------------------------------------------------------

# The following stages download container images for RKE2's Container Storage Interface (CSI).
# Each image is downloaded in its own dedicated build stage for maximum build efficiency.
#
# ARCHITECTURE RATIONALE:
#
# 1. **Parallel Builds:**
#    Docker BuildKit can build all these stages concurrently, significantly reducing
#    total build time. Without separate stages, images would download sequentially.
#
# 2. **Cache Granularity:**
#    Each image has independent caching. Updating ROOK_VERSION only invalidates the
#    Rook operator download—all other images remain cached. With sequential downloads
#    in a single stage, changing any version would invalidate all subsequent downloads.
#
# 3. **Isolated Failures:**
#    If one image download fails, it doesn't block progress on others. Retry is scoped
#    to just the failed stage.
#
# 4. **Clear Separation:**
#    Each stage has a single responsibility (download one image), making the Dockerfile
#    easier to understand and maintain.
#
# COMMAND CHAINING RATIONALE (why we use && despite builder stages):
#
# We chain commands with && (e.g., `skopeo copy ... && zstd ... && rm ...`) even though
# these are builder stages where intermediate layers don't affect final image size.
# This is intentional for several reasons:
#
# 1. **Disk Space During Build:**
#    The uncompressed .tar files can be large (Ceph is ~1.5GB). Chaining ensures they're
#    removed before the layer is committed, reducing the builder stage's disk footprint.
#
# 2. **BuildKit Cache Size:**
#    BuildKit stores layer caches on disk. Removing intermediate files keeps the cache
#    smaller and improves build performance on cache-constrained systems.
#
# 3. **Consistency:**
#    Following the same pattern everywhere makes the Dockerfile predictable and easier
#    to understand—no need to remember which stages chain and which don't.
#
# 4. **Layer Count:**
#    Fewer layers means faster layer export/import when using `docker save`/`docker load`
#    or pushing to registries (though this is a minor benefit for builder stages).

# ------------------------------------------------------------------------------
# BUILDER STAGE - LOCAL PATH PROVISIONER IMAGE
# ------------------------------------------------------------------------------

# Download local-path-provisioner image for CSI driver in a separate stage.
# This allows Docker to cache this download independently from other images.
FROM builder-rke2-artifacts AS builder-image-local-path-provisioner

ARG ARCH
ARG LOCAL_PATH_PROVISIONER_VERSION

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN skopeo copy \
      docker://rancher/local-path-provisioner:${LOCAL_PATH_PROVISIONER_VERSION} \
      docker-archive:/tmp/local-path-provisioner.tar:rancher/local-path-provisioner:${LOCAL_PATH_PROVISIONER_VERSION} \
      && zstd -T0 -19 /tmp/local-path-provisioner.tar \
      -o "csi-images-local-path.linux-${ARCH}-${LOCAL_PATH_PROVISIONER_VERSION}.tar.zst" \
      && rm /tmp/local-path-provisioner.tar

# ------------------------------------------------------------------------------
# BUILDER STAGE - ROOK OPERATOR IMAGE
# ------------------------------------------------------------------------------

FROM builder-rke2-artifacts AS builder-image-rook-ceph

ARG ROOK_VERSION

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN skopeo copy \
      docker://rook/ceph:${ROOK_VERSION} \
      docker-archive:/tmp/rook-ceph.tar:rook/ceph:${ROOK_VERSION} \
      && zstd -T0 -19 /tmp/rook-ceph.tar \
      -o csi-images-rook-ceph.linux-amd64-${ROOK_VERSION}.tar.zst \
      && rm /tmp/rook-ceph.tar

# ------------------------------------------------------------------------------
# BUILDER STAGE - CEPH CLUSTER IMAGE
# ------------------------------------------------------------------------------

FROM builder-rke2-artifacts AS builder-image-ceph

ARG CEPH_VERSION

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN skopeo copy \
      docker://quay.io/ceph/ceph:${CEPH_VERSION} \
      docker-archive:/tmp/ceph.tar:quay.io/ceph/ceph:${CEPH_VERSION} \
      && zstd -T0 -19 /tmp/ceph.tar \
      -o csi-images-ceph.linux-amd64-${CEPH_VERSION}.tar.zst \
      && rm /tmp/ceph.tar

# ------------------------------------------------------------------------------
# BUILDER STAGE - CEPH CSI DRIVER IMAGE
# ------------------------------------------------------------------------------

FROM builder-rke2-artifacts AS builder-image-cephcsi

ARG CEPHCSI_VERSION

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN skopeo copy \
      docker://quay.io/cephcsi/cephcsi:${CEPHCSI_VERSION} \
      docker-archive:/tmp/cephcsi.tar:quay.io/cephcsi/cephcsi:${CEPHCSI_VERSION} \
      && zstd -T0 -19 /tmp/cephcsi.tar \
      -o csi-images-cephcsi.linux-amd64-${CEPHCSI_VERSION}.tar.zst \
      && rm /tmp/cephcsi.tar

# ------------------------------------------------------------------------------
# BUILDER STAGE - CSI PROVISIONER IMAGE
# ------------------------------------------------------------------------------

FROM builder-rke2-artifacts AS builder-image-csi-provisioner

ARG CSI_PROVISIONER_VERSION

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN skopeo copy \
      docker://registry.k8s.io/sig-storage/csi-provisioner:${CSI_PROVISIONER_VERSION} \
      docker-archive:/tmp/csi-provisioner.tar:registry.k8s.io/sig-storage/csi-provisioner:${CSI_PROVISIONER_VERSION} \
      && zstd -T0 -19 /tmp/csi-provisioner.tar \
      -o csi-images-csi-provisioner.linux-amd64-${CSI_PROVISIONER_VERSION}.tar.zst \
      && rm /tmp/csi-provisioner.tar

# ------------------------------------------------------------------------------
# BUILDER STAGE - CSI ATTACHER IMAGE
# ------------------------------------------------------------------------------

FROM builder-rke2-artifacts AS builder-image-csi-attacher

ARG CSI_ATTACHER_VERSION

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN skopeo copy \
      docker://registry.k8s.io/sig-storage/csi-attacher:${CSI_ATTACHER_VERSION} \
      docker-archive:/tmp/csi-attacher.tar:registry.k8s.io/sig-storage/csi-attacher:${CSI_ATTACHER_VERSION} \
      && zstd -T0 -19 /tmp/csi-attacher.tar \
      -o csi-images-csi-attacher.linux-amd64-${CSI_ATTACHER_VERSION}.tar.zst \
      && rm /tmp/csi-attacher.tar

# ------------------------------------------------------------------------------
# BUILDER STAGE - CSI RESIZER IMAGE
# ------------------------------------------------------------------------------

FROM builder-rke2-artifacts AS builder-image-csi-resizer

ARG CSI_RESIZER_VERSION

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN skopeo copy \
      docker://registry.k8s.io/sig-storage/csi-resizer:${CSI_RESIZER_VERSION} \
      docker-archive:/tmp/csi-resizer.tar:registry.k8s.io/sig-storage/csi-resizer:${CSI_RESIZER_VERSION} \
      && zstd -T0 -19 /tmp/csi-resizer.tar \
      -o csi-images-csi-resizer.linux-amd64-${CSI_RESIZER_VERSION}.tar.zst \
      && rm /tmp/csi-resizer.tar

# ------------------------------------------------------------------------------
# BUILDER STAGE - CSI SNAPSHOTTER IMAGE
# ------------------------------------------------------------------------------

FROM builder-rke2-artifacts AS builder-image-csi-snapshotter

ARG CSI_SNAPSHOTTER_VERSION

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN skopeo copy \
      docker://registry.k8s.io/sig-storage/csi-snapshotter:${CSI_SNAPSHOTTER_VERSION} \
      docker-archive:/tmp/csi-snapshotter.tar:registry.k8s.io/sig-storage/csi-snapshotter:${CSI_SNAPSHOTTER_VERSION} \
      && zstd -T0 -19 /tmp/csi-snapshotter.tar \
      -o csi-images-csi-snapshotter.linux-amd64-${CSI_SNAPSHOTTER_VERSION}.tar.zst \
      && rm /tmp/csi-snapshotter.tar

# ------------------------------------------------------------------------------
# BUILDER STAGE - CSI NODE DRIVER REGISTRAR IMAGE
# ------------------------------------------------------------------------------

FROM builder-rke2-artifacts AS builder-image-csi-node-driver-registrar

ARG CSI_NODE_DRIVER_REGISTRAR_VERSION

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN skopeo copy \
      docker://registry.k8s.io/sig-storage/csi-node-driver-registrar:${CSI_NODE_DRIVER_REGISTRAR_VERSION} \
      docker-archive:/tmp/csi-node-driver-registrar.tar:registry.k8s.io/sig-storage/csi-node-driver-registrar:${CSI_NODE_DRIVER_REGISTRAR_VERSION} \
      && zstd -T0 -19 /tmp/csi-node-driver-registrar.tar \
      -o csi-images-csi-node-driver-registrar.linux-amd64-${CSI_NODE_DRIVER_REGISTRAR_VERSION}.tar.zst \
      && rm /tmp/csi-node-driver-registrar.tar

# ------------------------------------------------------------------------------
# BUILDER STAGE - CSI ADDONS IMAGE
# ------------------------------------------------------------------------------

FROM builder-rke2-artifacts AS builder-image-csi-addons

ARG CSI_ADDONS_VERSION

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN skopeo copy \
      docker://quay.io/csiaddons/k8s-sidecar:${CSI_ADDONS_VERSION} \
      docker-archive:/tmp/csi-addons.tar:quay.io/csiaddons/k8s-sidecar:${CSI_ADDONS_VERSION} \
      && zstd -T0 -19 /tmp/csi-addons.tar \
      -o csi-images-csi-addons.linux-amd64-${CSI_ADDONS_VERSION}.tar.zst \
      && rm /tmp/csi-addons.tar

# ------------------------------------------------------------------------------
# BUILDER STAGE - CEPH CSI OPERATOR IMAGE
# ------------------------------------------------------------------------------

FROM builder-rke2-artifacts AS builder-image-ceph-csi-operator

ARG CEPH_CSI_OPERATOR_VERSION

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

RUN skopeo copy \
      docker://quay.io/cephcsi/ceph-csi-operator:${CEPH_CSI_OPERATOR_VERSION} \
      docker-archive:/tmp/ceph-csi-operator.tar:quay.io/cephcsi/ceph-csi-operator:${CEPH_CSI_OPERATOR_VERSION} \
      && zstd -T0 -19 /tmp/ceph-csi-operator.tar \
      -o csi-images-ceph-csi-operator.linux-amd64-${CEPH_CSI_OPERATOR_VERSION}.tar.zst \
      && rm /tmp/ceph-csi-operator.tar

# ------------------------------------------------------------------------------
# BUILDER STAGE - RKE2 CEPH ARTIFACTS
# ------------------------------------------------------------------------------

# Build RKE2 Ceph variant artifacts by copying image tarballs from separate build stages.
# Each image is built in parallel in its own stage, improving build performance and cache granularity.
FROM builder-rke2-artifacts AS builder-rke2-ceph-artifacts

ARG ARCH
ARG ROOK_VERSION
ARG CEPH_VERSION
ARG CEPHCSI_VERSION
ARG CSI_PROVISIONER_VERSION
ARG CSI_ATTACHER_VERSION
ARG CSI_RESIZER_VERSION
ARG CSI_SNAPSHOTTER_VERSION
ARG CSI_NODE_DRIVER_REGISTRAR_VERSION
ARG CSI_ADDONS_VERSION
ARG CEPH_CSI_OPERATOR_VERSION

# Enable strict error handling.
SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

# Copy all image tarballs from their respective build stages.
# These were built in parallel, so this is just collecting the results.
COPY --from=builder-image-rook-ceph /build/resources/rke2/csi-images-rook-ceph.*.tar.zst csi/images/
COPY --from=builder-image-ceph /build/resources/rke2/csi-images-ceph.*.tar.zst csi/images/
COPY --from=builder-image-cephcsi /build/resources/rke2/csi-images-cephcsi.*.tar.zst csi/images/
COPY --from=builder-image-csi-provisioner /build/resources/rke2/csi-images-csi-provisioner.*.tar.zst csi/images/
COPY --from=builder-image-csi-attacher /build/resources/rke2/csi-images-csi-attacher.*.tar.zst csi/images/
COPY --from=builder-image-csi-resizer /build/resources/rke2/csi-images-csi-resizer.*.tar.zst csi/images/
COPY --from=builder-image-csi-snapshotter /build/resources/rke2/csi-images-csi-snapshotter.*.tar.zst csi/images/
COPY --from=builder-image-csi-node-driver-registrar /build/resources/rke2/csi-images-csi-node-driver-registrar.*.tar.zst csi/images/
COPY --from=builder-image-csi-addons /build/resources/rke2/csi-images-csi-addons.*.tar.zst csi/images/
COPY --from=builder-image-ceph-csi-operator /build/resources/rke2/csi-images-ceph-csi-operator.*.tar.zst csi/images/

# Download Rook v1.20.6 manifest files directly from GitHub.
# These are the official Rook quickstart manifests, used unchanged.
# Deployment order: crds -> common -> csi-operator -> operator -> cluster -> pool -> storageclass
RUN mkdir -p csi/manifests/
RUN curl -sL https://raw.githubusercontent.com/rook/rook/${ROOK_VERSION}/deploy/examples/crds.yaml -o csi/manifests/rook-crds.yaml \
      && curl -sL https://raw.githubusercontent.com/rook/rook/${ROOK_VERSION}/deploy/examples/common.yaml -o csi/manifests/rook-common.yaml \
      && curl -sL https://raw.githubusercontent.com/rook/rook/${ROOK_VERSION}/deploy/examples/csi-operator.yaml -o csi/manifests/rook-csi-operator.yaml \
      && curl -sL https://raw.githubusercontent.com/rook/rook/${ROOK_VERSION}/deploy/examples/operator.yaml -o csi/manifests/rook-operator.yaml \
      && curl -sL https://raw.githubusercontent.com/rook/rook/${ROOK_VERSION}/deploy/examples/cluster.yaml -o csi/manifests/rook-cluster.yaml \
      && curl -sL https://raw.githubusercontent.com/rook/rook/${ROOK_VERSION}/deploy/examples/pool.yaml -o csi/manifests/rook-pool.yaml

# Copy custom StorageClass (based on official example, modified to be default).
COPY resources/resources/rook-storageclass.yaml csi/manifests/

# Remove the local-path CSI manifest.
RUN rm -f csi/manifests/csi-local-path-provisioner.yaml

# Update VERSION.json to reflect Ceph variant.
RUN cat > VERSION.json <<EOF
{
  "ironbark": "rke2-ceph-variant",
  "tools": {
    "rke2": "${RKE2_VERSION}",
    "rook": "${ROOK_VERSION}",
    "ceph": "${CEPH_VERSION}",
    "cephcsi": "${CEPHCSI_VERSION}",
    "csi_provisioner": "${CSI_PROVISIONER_VERSION}",
    "csi_attacher": "${CSI_ATTACHER_VERSION}",
    "csi_resizer": "${CSI_RESIZER_VERSION}",
    "csi_snapshotter": "${CSI_SNAPSHOTTER_VERSION}",
    "csi_node_driver_registrar": "${CSI_NODE_DRIVER_REGISTRAR_VERSION}"
  }
}
EOF

# ------------------------------------------------------------------------------
# BUILDER STAGE - DOCUMENTATION
# ------------------------------------------------------------------------------

# Dedicated stage for generating HTML and PDF documentation from README.adoc.
# This keeps documentation tooling (Ruby, asciidoctor) isolated from other
# builder stages and allows parallel builds.
#
# Output Location: /build/docs/README.html and /build/docs/README.pdf
# These files are copied to /app/resources/docs/ in the final image stages.
# See coupling with: internal/api/handlers_docs.go:handleDocsReadme()
FROM ${UBUNTU_IMAGE} AS builder-documentation

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]

# Install Ruby and asciidoctor tooling for documentation generation.
RUN apt-get update \
      && apt-get install -y --no-install-recommends \
        ruby \
        ruby-dev \
        make \
        gcc \
        g++ \
      && gem install asciidoctor asciidoctor-pdf --no-document \
      && apt-get clean \
      && rm -rf /var/lib/apt/lists/*

WORKDIR /build/docs

# Copy only the README for documentation generation.
COPY README.adoc .

# Generate HTML and PDF documentation.
# Output: README.html and README.pdf in /build/docs/
RUN asciidoctor -b html5 -o README.html README.adoc
RUN asciidoctor-pdf -o README.pdf README.adoc

# ------------------------------------------------------------------------------
# BUILDER STAGE - GOLANG
# ------------------------------------------------------------------------------

FROM golang:${GOLANG_VERSION} AS builder-golang

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

# NOTE: ARG declarations are placed AFTER dependency and source copy operations
# to preserve Docker layer cache. Build metadata (APP_VERSION, BUILD_DATE, etc.)
# changes on every build and would invalidate all subsequent layers if declared
# at stage start. These ARGs are only needed for the final `make build` command.
ARG APP_VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_TIME_UTC=unknown
ARG BUILD_DATE=unknown

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

# Copy documentation for serving via API.
# The README.html is served at GET /ironbark and GET / (redirect).
# Location: /app/resources/docs/README.html
# This path MUST match the readmePath in internal/api/handlers_docs.go:handleDocsReadme()
# If this path changes, update the handler code accordingly.
RUN mkdir -p /app/resources/docs
COPY --from=builder-documentation /build/docs/README.html /app/resources/docs/
COPY --from=builder-documentation /build/docs/README.pdf /app/resources/docs/

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

ARG ARCH
ARG LOCAL_PATH_PROVISIONER_VERSION

# Set environment variable to indicate RKE2 variant.
ENV IRONBARK_RKE2_AVAILABLE=true

# Copy RKE2 artifacts from the RKE2 builder stage to /app/resources/rke2.
# This path is consistent with other resources and used by the artifact download API.
COPY --from=builder-rke2-artifacts /build/resources/rke2/ resources/rke2/
COPY --from=builder-image-local-path-provisioner /build/resources/rke2/csi-images-local-path.linux-${ARCH}-${LOCAL_PATH_PROVISIONER_VERSION}.tar.zst resources/rke2/csi/images/

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
