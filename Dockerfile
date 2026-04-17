FROM golang:1.26.2@sha256:5f3787b7f902c07c7ec4f3aa91a301a3eda8133aa32661a3b3a3a86ab3a68a36

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.14@sha256:42ecfb253183ec823646dd7859c5652039669409b44daa72abf57112e622849a /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.15.3@sha256:69d129ef9463130cf903a638e74a8e41193ff15a2e25010d1bbb5ba97f5f3762 /usr/bin/goreleaser /usr/local/bin/goreleaser

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
