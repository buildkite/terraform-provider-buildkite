FROM golang:1.27.0@sha256:4013ae0f9e7994f8535c58c811f8f863fbed38b72e0d51e6592156f758d66146

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.16@sha256:64360659224d6cbeb099eeed61aa66a80e02c18ba08c0243bd905165b47b088e /bin/terraform /usr/local/bin/terraform
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
