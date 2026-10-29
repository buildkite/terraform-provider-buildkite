default: build

build:
	go build -o terraform-provider-buildkite -ldflags="-s -w -X main.version=$(shell git describe --tag)" .

.PHONY: build-snapshot
build-snapshot:
	goreleaser build --snapshot --clean

.PHONY: docs
docs:
	go get github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs

testfmt:
	@test -z $(shell gofmt -l . buildkite | tee /dev/stderr)

vet:
	go vet $(go list ./...)

.PHONY: test testacc

# Keep live TestAcc tests out of the unit job, even if TF_ACC was inherited.
test:
	TF_ACC= go test ./... -parallel=4 -skip '^TestAcc'

# Acceptance tests. This will create, manage and delete real resources in a real
# Buildkite organization!
testacc:
	TF_ACC=1 go run gotest.tools/gotestsum --format testname --junitfile "junit-${BUILDKITE_JOB_ID}.xml" --jsonfile "test-events-${BUILDKITE_JOB_ID}.jsonl" -- -count=1 -parallel=12 -run '^TestAcc' ./...

# Generate the Buildkite GraphQL schema file
schema:
	go tool gqlfetch -endpoint "$${BUILDKITE_GRAPHQL_URL:-https://graphql.buildkite.com/v1}" -header "Authorization=Bearer $${BUILDKITE_API_TOKEN:-$$BUILDKITE_GRAPHQL_TOKEN}" > schema.graphql

# Generate the GraphQL code
generate: schema
	go tool genqlient
