FROM golang:1.26.4@sha256:b4f9f8a927c6e8f24da1b653f0c7ca86fd1677fe371b03f69fd416166b527268

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.15@sha256:a12a7a9301bbab26589c0a353d5bdfc68bd1a52aa818cbdd698bf0dec094bd61 /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.15.4@sha256:579eee23514fa647adcc669b5875f866f1c1faf5a0464aec4614a9121684c06c /usr/bin/goreleaser /usr/local/bin/goreleaser

WORKDIR /work

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./

# Download dependencies - this layer will be cached unless
# go.mod/go.sum changes
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Configure build caching
ENV GOCACHE=/root/.cache/go-build
