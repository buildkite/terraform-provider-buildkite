FROM golang:1.27.1@sha256:512690a5660563b57d37ecc31129e7f136e831db2aed24a1dbeb8ad7380dc0fa

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.16@sha256:f4d9594d2c8010c03f0149352682166410c58c21d46344cf256fd5a4b721a011 /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.18.0@sha256:a7609141326e383370858ab3ca2572e96e00fb212fe3fd5610cd4de434652faa /usr/bin/goreleaser /usr/local/bin/goreleaser

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
