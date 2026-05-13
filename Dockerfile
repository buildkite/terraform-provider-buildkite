FROM golang:1.26.3@sha256:633d23bf362cb40dd72b4f277288a8929697d77537f9c801b81aeced19b5bdf3

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.15@sha256:ffbb410cccfdb89f8d2d4d3b40abc171db747c6d2c6ee9a3ce0e10d1e838a41e /bin/terraform /usr/local/bin/terraform
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
