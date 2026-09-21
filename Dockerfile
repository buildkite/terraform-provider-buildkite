FROM golang:1.27.1@sha256:f44f6e88636cfb311f9ebace870ded69d943f227bb3cb27d32ffd84ea18c43ea

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.16@sha256:c9a9d991c113f3bda5269de1506983d45ce1409dfe702df433acf10a5ea9f6bc /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.18.2@sha256:7077423cf5ef643ff56a34b58f93c1364e927e5c3dfa470eeabc44cab1a9c72b /usr/bin/goreleaser /usr/local/bin/goreleaser

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
