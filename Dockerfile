FROM golang:1.26.0@sha256:b75179794e029c128d4496f695325a4c23b29986574ad13dd52a0d3ee9f72a6f

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.14@sha256:96d2bc440714bf2b2f2998ac730fd4612f30746df43fca6f0892b2e2035b11bc /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.14.1@sha256:4cb6c58e37990fe9e08221afc8cef8c9b596d35972be863ca8ec7ed54c3c8654 /usr/bin/goreleaser /usr/local/bin/goreleaser

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
