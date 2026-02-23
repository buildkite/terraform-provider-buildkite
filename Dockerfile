FROM golang:1.26.0@sha256:c83e68f3ebb6943a2904fa66348867d108119890a2c6a2e6f07b38d0eb6c25c5

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.14@sha256:96d2bc440714bf2b2f2998ac730fd4612f30746df43fca6f0892b2e2035b11bc /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.13.3@sha256:846b7cad015f87712ad4e68d9cd59bfde4e59c9970265f76dd4e3b3b6f41c768 /usr/bin/goreleaser /usr/local/bin/goreleaser

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
