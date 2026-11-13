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

GOTESTSUM ?= go run gotest.tools/gotestsum@v1.13.0

.PHONY: test testacc testacc-nonadmin

# Keep live TestAcc tests out of the unit job, even if TF_ACC was inherited.
test:
	TF_ACC= go test ./... -parallel=4 -skip '^TestAcc'

# Acceptance tests. This will create, manage and delete real resources in a real
# Buildkite organization!
testacc:
	TF_ACC=1 $(GOTESTSUM) --format testname --junitfile "junit-${BUILDKITE_JOB_ID}.xml" --jsonfile "test-events-${BUILDKITE_JOB_ID}.jsonl" -- -count=1 -parallel=12 -run '^TestAcc' ./...

# Shared lifecycles and lookups using a team maintainer's token with graphql,
# write_pipelines, read_clusters, read_suites, write_suites, read_secrets_details,
# and write_secrets scopes.
# Fail rather than silently succeed if the selected tests have been removed.
testacc-nonadmin:
	@tests="$$(TF_ACC= go test ./buildkite -list '^TestAccNonAdmin')" || exit $$?; \
		printf '%s\n' "$$tests" | grep -q '^TestAccNonAdmin' || { echo 'No non-admin acceptance tests found' >&2; exit 1; }
	TF_ACC=1 BUILDKITE_ACCEPTANCE_NONADMIN=1 $(GOTESTSUM) --format testname --junitfile "junit-${BUILDKITE_JOB_ID}.xml" --jsonfile "test-events-${BUILDKITE_JOB_ID}.jsonl" -- -count=1 -parallel=12 -run '^TestAccNonAdmin' ./buildkite

# Destructive: remove marked acceptance resources strictly older than 24 hours.
# The organization is passed as the sweeper's region; ordinary tests do not run.
.PHONY: sweep
sweep:
	@test -n "$(BUILDKITE_ORGANIZATION_SLUG)" || { echo 'BUILDKITE_ORGANIZATION_SLUG is required' >&2; exit 1; }
	@test -n "$$BUILDKITE_API_TOKEN" || { echo 'BUILDKITE_API_TOKEN is required' >&2; exit 1; }
	TF_ACC= go test ./buildkite -v -count=1 -run '^$$' -timeout=30m -sweep="$(BUILDKITE_ORGANIZATION_SLUG)"

# Generate the Buildkite GraphQL schema file
schema:
	go tool gqlfetch -endpoint "$${BUILDKITE_GRAPHQL_URL:-https://graphql.buildkite.com/v1}" -header "Authorization=Bearer $${BUILDKITE_API_TOKEN:-$$BUILDKITE_GRAPHQL_TOKEN}" > schema.graphql

# Generate the GraphQL code
generate: schema
	go tool genqlient
