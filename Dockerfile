FROM golang:1.25.6@sha256:06d1251c59a75761ce4ebc8b299030576233d7437c886a68b43464bad62d4bb1

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.4@sha256:4dcb45513699e652c771b914f41ec1cc2a0ba9c8d1afa2e8e4aa2ba071b63151 /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v1.26.2@sha256:e69fcf552e8eb2ce0d4c4a9b080b5f82ad9f040bb039a203667db0b5274ebfc3 /usr/bin/goreleaser /usr/local/bin/goreleaser

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
