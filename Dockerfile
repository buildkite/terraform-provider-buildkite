FROM golang:1.26.0@sha256:fb612b7831d53a89cbc0aaa7855b69ad7b0caf603715860cf538df854d047b84

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.14@sha256:47767069b0be3969e5dfaf1c5b01030ce4510002dec43b68380ba7c0799c7f31 /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v2.14.2@sha256:f1ec85ab5d4fef29ed5f948ebb25afccb12eb6df98ae3228eb111cb36199e1e9 /usr/bin/goreleaser /usr/local/bin/goreleaser

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
