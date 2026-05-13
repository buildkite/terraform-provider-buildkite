FROM golang:1.26.3@sha256:313faae491b410a35402c05d35e7518ae99103d957308e940e1ae2cfa0aac29b

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.15@sha256:a12a7a9301bbab26589c0a353d5bdfc68bd1a52aa818cbdd698bf0dec094bd61 /bin/terraform /usr/local/bin/terraform
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
