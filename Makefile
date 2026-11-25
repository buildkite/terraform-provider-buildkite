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

# Organization-setting writers, their public-pipeline readers, and tests using
# the persistent Rule fixture's trigger slot must share one exclusive CI step.
TESTACC_SHARED = ^(TestAccBuildkiteOrganizationResource|TestAccBuildkiteOrganizationIPEntries|TestAccBuildkiteOrganizationBrowserEntries|TestAccBuildkiteOrganizationPipelineSettingsResource|TestAccBuildkiteOrganizationBannerResource|TestAccBuildkitePipelinePublicVisibility|TestAccNonAdminOrganizationRuleCreate|TestAccBuildkiteOrganizationRuleMutationPermissions)$$
TESTACC_SKIP = ^$$

.PHONY: test testacc testacc-isolated testacc-nonadmin testacc-nonadmin-isolated testacc-shared

# Keep live TestAcc tests out of the unit job, even if TF_ACC was inherited.
test:
	TF_ACC= go test ./... -parallel=4 -skip '^TestAcc'

# Acceptance tests. This will create, manage and delete real resources in a real
# Buildkite organization!
# The original targets retain full coverage for local use; CI skips shared tests.
testacc-isolated: TESTACC_SKIP = $(TESTACC_SHARED)
testacc testacc-isolated:
	TF_ACC=1 $(GOTESTSUM) --format testname --junitfile "junit-${BUILDKITE_JOB_ID}.xml" --jsonfile "test-events-${BUILDKITE_JOB_ID}.jsonl" -- -count=1 -parallel=12 -run '^TestAcc' -skip '$(TESTACC_SKIP)' ./...

# Shared lifecycles and lookups using a team maintainer's token with graphql,
# write_pipelines, read_clusters, read_suites, write_suites, read_secrets_details,
# and write_secrets scopes.
# Fail rather than silently succeed if the selected tests have been removed.
testacc-nonadmin-isolated: TESTACC_SKIP = $(TESTACC_SHARED)
testacc-nonadmin testacc-nonadmin-isolated:
	@tests="$$(TF_ACC= go test ./buildkite -list '^TestAccNonAdmin')" || exit $$?; \
		printf '%s\n' "$$tests" | grep '^TestAccNonAdmin' | grep -Ev '$(TESTACC_SKIP)' >/dev/null || { echo 'No non-admin acceptance tests found' >&2; exit 1; }
	TF_ACC=1 BUILDKITE_ACCEPTANCE_NONADMIN=1 $(GOTESTSUM) --format testname --junitfile "junit-${BUILDKITE_JOB_ID}.xml" --jsonfile "test-events-${BUILDKITE_JOB_ID}.jsonl" -- -count=1 -parallel=12 -run '^TestAccNonAdmin' -skip '$(TESTACC_SKIP)' ./buildkite

# Run both credential selections even if one fails, without overwriting reports.
# Go's parallel limit also serializes shared tests that use resource.ParallelTest.
testacc-shared:
	@test -n "$$BUILDKITE_NONADMIN_API_TOKEN" || { echo 'BUILDKITE_NONADMIN_API_TOKEN is required' >&2; exit 1; }
	@status=0; \
		TF_ACC=1 $(GOTESTSUM) --format testname --junitfile "junit-${BUILDKITE_JOB_ID}-shared-admin.xml" --jsonfile "test-events-${BUILDKITE_JOB_ID}-shared-admin.jsonl" -- -count=1 -parallel=1 -run '$(TESTACC_SHARED)' ./buildkite || status=$$?; \
		TF_ACC=1 BUILDKITE_API_TOKEN="$$BUILDKITE_NONADMIN_API_TOKEN" BUILDKITE_ACCEPTANCE_NONADMIN=1 $(GOTESTSUM) --format testname --junitfile "junit-${BUILDKITE_JOB_ID}-shared-nonadmin.xml" --jsonfile "test-events-${BUILDKITE_JOB_ID}-shared-nonadmin.jsonl" -- -count=1 -parallel=1 -run '^TestAccNonAdminOrganizationRuleCreate$$' ./buildkite || status=$$?; \
		exit $$status

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
