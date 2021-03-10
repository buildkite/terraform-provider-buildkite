default: build

build:
	go build -o terraform-provider-buildkite .

fmt:
	go fmt ./...

testfmt:
	@test -z $(shell gofmt -l . buildkite | tee /dev/stderr)

vet:
	go vet $(go list ./...)

test:
	go test ./...

# Acceptance tests. This will create, manage and delete real resources in a real
# Buildkite organization!
testacc:
	TF_ACC=1 go test -v ./...

# Acceptance tests, but only the ones that can pass with a non-admin API token. Non-admins can manage
# pipelines and pipeline schedules, but only if they use teams. The API token must also belong to a user
# who is a maintainer of the team.
#
# This will create, manage and delete real resources in a real Buildkite organization!
testacc-nonadmin:
	TF_ACC=1 go test -v -run "TestAccPipeline(Schedule)?_.*withteams" ./...
