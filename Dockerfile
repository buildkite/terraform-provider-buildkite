FROM golang:1.26.1@sha256:595c7847cff97c9a9e76f015083c481d26078f961c9c8dca3923132f51fe12f1

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.14@sha256:42ecfb253183ec823646dd7859c5652039669409b44daa72abf57112e622849a /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.15.1@sha256:c3c61ebcc4dac1981624ae1436220bd5ce808cb932edad7d5ff379a217815a8a /usr/bin/goreleaser /usr/local/bin/goreleaser

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
