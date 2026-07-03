FROM golang:1.26.4@sha256:792443b89f65105abba56b9bd5e97f680a80074ac62fc844a584212f8c8102c3

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.15@sha256:40e61a86763083ea987ded0ffa15f6d75e0df48ed16275811f949b3ecbcd8aae /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.16.0@sha256:f5ce92e9a38fb9406ccd638b95e43402cd3f4c567cb677eb06af9fd161278c12 /usr/bin/goreleaser /usr/local/bin/goreleaser

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
