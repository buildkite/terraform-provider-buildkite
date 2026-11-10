FROM golang:1.27.3@sha256:f24164268fc1e741bbdc0066dbadcf1c8426c5a8489f0729c02c3905a53032fd AS base

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.16@sha256:bfffc1ded5fbf0a17beaaf22eb4793980abdb19d0761ff6a3a0efb2ab4d9b316 /bin/terraform /usr/local/bin/terraform

WORKDIR /work
ENV GOCACHE=/root/.cache/go-build

# Modules and compiled packages are cached in Compose's runtime volumes. A build
# cache mount here would not populate those volumes and would download twice.

FROM base AS test
ARG TARGETARCH
# Official v1.13.0 release archives, verified against its published checksums.
# Keep the version, archive names and checksums together when upgrading.
RUN set -eu; \
    case "$TARGETARCH" in \
        amd64) checksum=11ccddeaf708ef228889f9fe2f68291a75b27013ddfc3b18156e094f5f40e8ee ;; \
        arm64) checksum=7644a4c5cd1bb978d56245aeab25a586ac5ac62adebed20a399548867c13499d ;; \
        *) echo "Unsupported gotestsum architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl --fail --show-error --silent --location --retry 3 \
        "https://github.com/gotestyourself/gotestsum/releases/download/v1.13.0/gotestsum_1.13.0_linux_${TARGETARCH}.tar.gz" \
        --output /tmp/gotestsum.tar.gz; \
    echo "$checksum  /tmp/gotestsum.tar.gz" | sha256sum --check --strict; \
    tar -xzf /tmp/gotestsum.tar.gz -C /usr/local/bin gotestsum; \
    rm /tmp/gotestsum.tar.gz

# Preserve the default image's snapshot/release tooling for existing callers.
FROM base AS release
COPY --from=goreleaser/goreleaser:v2.18.3@sha256:b61a4eedf5e3b1d283792dade34ee0f8c6cce3e6f1f65851d3b397b8f8a75b6d /usr/bin/goreleaser /usr/local/bin/goreleaser
