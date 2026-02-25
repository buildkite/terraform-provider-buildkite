FROM golang:1.26.0@sha256:b75179794e029c128d4496f695325a4c23b29986574ad13dd52a0d3ee9f72a6f

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.14@sha256:96d2bc440714bf2b2f2998ac730fd4612f30746df43fca6f0892b2e2035b11bc /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.14.0@sha256:3a5ce6c3f66984c97b67a70bddf66fa26a99e1b6278d29da91bc204c6a574ab1 /usr/bin/goreleaser /usr/local/bin/goreleaser

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
