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

ARG ARCH=amd64
ARG UBUNTU_IMAGE=ubuntu:24.04
ARG GOLANG_VERSION=1.25.6

# Tool versions.
ARG KUBECTL_VERSION=v1.34.1
ARG ZARF_VERSION=v0.64.0

# ------------------------------------------------------------------------------
# BUILDER STAGE - TOOLING
# ------------------------------------------------------------------------------

FROM ${UBUNTU_IMAGE} AS builder-tooling

ARG ARCH
ARG KUBECTL_VERSION
ARG ZARF_VERSION

RUN \
  apt -y update \
  && apt -y install \
    # CAs must be up to date since `zarf package create` downloads from https sites.
    ca-certificates

WORKDIR /build/bin
ADD \
  https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl \
  kubectl
RUN chmod +x kubectl

ADD \
  https://github.com/zarf-dev/zarf/releases/download/${ZARF_VERSION}/zarf_${ZARF_VERSION}_Linux_amd64 \
  zarf
RUN chmod +x zarf

# Make a self-contained directory which can be used to do `zarf init`.
# This allows a single directory to be copied onto host if installing k3s.
WORKDIR /build/resources/zarf/init
# Hard link to save space.
RUN ln /build/bin/zarf
# Explicitly `ADD` to allow better Docker caching (rather than `zarf tools download-init`).
ADD \
  # URL from: https://docs.zarf.dev/best-practices/upgrading-zarf/
  https://github.com/zarf-dev/zarf/releases/download/${ZARF_VERSION}/zarf-init-${ARCH}-${ZARF_VERSION}.tar.zst \
  zarf-init-${ARCH}-${ZARF_VERSION}.tar.zst

# Build each package.
WORKDIR /src/zarf/packages
COPY resources/packages/ .
RUN \
  for package_type in *; do \
    for package_dir in ${package_type}/*; do \
      /build/bin/zarf package create "${package_dir}" -o /build/resources/packages/${package_type}/; \
    done \
  done

# ------------------------------------------------------------------------------
# BUILDER STAGE - GOLANG
# ------------------------------------------------------------------------------

FROM golang:${GOLANG_VERSION} AS builder-golang
WORKDIR /build
COPY src/go.mod src/go.sum ./
RUN go mod download
COPY ./src/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o ironbark .

# ------------------------------------------------------------------------------
# FINAL STAGE
# ------------------------------------------------------------------------------

FROM  ${UBUNTU_IMAGE}

ARG IRONBARK_DATA_DIR=/mnt/data
ARG IRONBARK_INTERNAL_PACKAGES_DIR=/app/resources/packages
ENV \
  IRONBARK_DATA_DIR=${IRONBARK_DATA_DIR} \
  IRONBARK_INTERNAL_PACKAGES_DIR=${IRONBARK_INTERNAL_PACKAGES_DIR} \
  PATH="/app/bin:${PATH}"

WORKDIR /app
COPY --from=builder-tooling /build/ .
COPY --from=builder-golang /build/ironbark bin/

RUN ls -lR /app
ENTRYPOINT ["/app/bin/ironbark"]

ARG BUILD_DATE
ARG VCS_REF
LABEL org.label-schema.name="nswcc-deployment" \
      org.label-schema.description="Image used for k8s deployment" \
      org.opencontainers.image.authors="brightSPARK Labs <enquire@brightsparklabs.com>" \
      org.label-schema.vendor="brightSPARK Labs" \
      org.label-schema.schema-version="1.0.0-rc1" \
      org.label-schema.vcs-url="https://bitbucket.org/brightsparklabs/nswcc-deployment" \
      org.label-schema.vcs-ref=${VCS_REF} \
      org.label-schema.build-date=${BUILD_DATE}
ENV \
  META_BUILD_DATE=${BUILD_DATE} \
  META_VCS_REF=${VCS_REF}
