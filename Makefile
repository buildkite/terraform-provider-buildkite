default: build

build:
	go build -o terraform-provider-buildkite -ldflags="-s -w -X main.version=$(shell git describe --tag)" .

.PHONY: build-snapshot
build-snapshot:
	goreleaser build --snapshot --clean

.PHONY: docs
docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --tf-version 1.9.8

testfmt:
	@test -z $(shell gofmt -l . buildkite | tee /dev/stderr)

vet:
	go vet $(go list ./...)

test:
	go test ./...

# Acceptance tests. This will create, manage and delete real resources in a real
# Buildkite organization!
testacc:
	TF_ACC=1 go run gotest.tools/gotestsum --format testname --junitfile "junit-${BUILDKITE_JOB_ID}.xml" ./...

# Generate the Buildkite GraphQL schema file
schema:
	go get github.com/suessflorian/gqlfetch/gqlfetch
	go get github.com/Khan/genqlient/generate@v0.7.0
	go get github.com/vektah/gqlparser/v2/validator@v2.5.15
	go run github.com/suessflorian/gqlfetch/gqlfetch -endpoint https://graphql.buildkite.com/v1 -header "Authorization=Bearer $${BUILDKITE_GRAPHQL_TOKEN}" > schema.graphql

# Generate the GraphQL code
generate: schema
	go run github.com/Khan/genqlient
