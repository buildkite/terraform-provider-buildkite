FROM golang:1.26.1@sha256:595c7847cff97c9a9e76f015083c481d26078f961c9c8dca3923132f51fe12f1

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.14@sha256:42ecfb253183ec823646dd7859c5652039669409b44daa72abf57112e622849a /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.15.2@sha256:5be644c8c779677d069b7f50d5e655274c65b5e188c41268abd5b3713c416527 /usr/bin/goreleaser /usr/local/bin/goreleaser

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
