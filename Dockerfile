FROM golang:1.26.4@sha256:87a41d2539e5671777734e91f467499ed5eafb1fb1f77221dff2744db7a51775

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.15@sha256:adae45661e45d3c88beef071ee1277b4621cea73517aae7f0844657c8e85f641 /bin/terraform /usr/local/bin/terraform
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
