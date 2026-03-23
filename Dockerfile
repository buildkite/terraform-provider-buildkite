FROM golang:1.26.1@sha256:595c7847cff97c9a9e76f015083c481d26078f961c9c8dca3923132f51fe12f1

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.14@sha256:880e7eeb8da8be56de88fbcd2576b5776f140a829c60e22154c2e43d67564c4e /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.14.3@sha256:848430a900a83ca0e18f2f149fb4ddcdaea74a667aa07224268b97d448833591 /usr/bin/goreleaser /usr/local/bin/goreleaser

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
