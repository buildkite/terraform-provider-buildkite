FROM golang:1.27.1@sha256:f44f6e88636cfb311f9ebace870ded69d943f227bb3cb27d32ffd84ea18c43ea

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.16@sha256:c3308fcbb530627c102c4c4b993e226b202429084328c3cc505cc2a69342e883 /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.18.1@sha256:92b918cc587dce6321b5fafc57ba93942a38592a7fbdb6cc3e300418b9f03a7e /usr/bin/goreleaser /usr/local/bin/goreleaser

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
