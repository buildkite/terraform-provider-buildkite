FROM golang:1.26.5@sha256:079e59808d2d252516e27e3f3a9c003740dee7f75e55aa71528766d52bcfc16a

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.15@sha256:7ae513256f7ce67879e218ae8593d6fbe216ec9e123abe6c94e4e10704857963 /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.17.0@sha256:054eefd282c02233a2556ce2d1a60cd2f51dc565ffc2520dc38b5deb4dd1ad30 /usr/bin/goreleaser /usr/local/bin/goreleaser

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
