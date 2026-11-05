# Contributing to Buildkite Terraform Provider

We welcome contributions from the community to make the Buildkite Terraform Provider even better.

## Getting Started

To get started with contributing, please follow these steps:

1. Fork the repository
2. Create a feature branch with a descriptive name (`git checkout -b my-new-feature`)
3. Write your code
4. Commit your changes and push them to your forked repository
5. Submit a pull request with a detailed description of your changes

The Buildkite team will review your PR and start a CI build for it. The team aims to interact with every PR within a week. For security reasons, CI isn't automatically run on forked repos, and a human will review the PR before CI runs.

## Development Prerequisites

You'll need the following installed on your machine:

- **Go 1.26.10+ (1.26 series) or 1.27.3+**: Use a patched toolchain for the eMudhra intermediate CA compromise (CVE-2026-111943). Development and release builds use Go 1.27.3.
- **Terraform**: For testing your changes
- **Lefthook** (optional): For managing Git hooks (see [lefthook.dev](https://lefthook.dev))

Dependencies are managed via Go modules and installed automatically as required.

## Building the Provider

To compile the provider:

```bash
make build
```

This will create a `terraform-provider-buildkite` binary in the current directory.

To build release binaries for all platforms:

```bash
make build-snapshot
```

## Running Tests

### Unit Tests

Run tests that do not use the live Buildkite API:

```bash
make test
```

Top-level tests named `TestAcc…` belong to the live acceptance suite. `make test`
clears `TF_ACC` and excludes those tests; `make testacc` runs only those tests.
Keep fake-API and configuration-validation tests in the unit suite, using
`resource.UnitTest` rather than `resource.Test` or `resource.ParallelTest`.
`TestHarnessPartition` checks this convention for direct Terraform harness calls.
Terraform-based unit tests need a Terraform executable and may download one if it
is not installed.

### Acceptance Tests

Acceptance tests test the provider against the live Buildkite API:

```bash
BUILDKITE_ORGANIZATION_SLUG=<org-slug> BUILDKITE_API_TOKEN=<token> make testacc
```

**Important notes about acceptance tests:**

- These tests make live changes to an organization and probably shouldn't be run against organizations with real data
- Anyone actively developing features for this provider is welcome to request a test organization by contacting support@buildkite.com
- The CI process will not run acceptance tests on pull requests
- Code reviewers will run the acceptance tests manually
- Please run the acceptance tests locally to confirm they pass before requesting a review

#### Non-admin acceptance tests

Run the shared pipeline and pipeline schedule lifecycle tests and read-only data
source tests with a non-admin token:

```bash
BUILDKITE_ORGANIZATION_SLUG=<test-org> BUILDKITE_API_TOKEN=<non-admin-token> \
BUILDKITE_NONADMIN_RULE_UUID=<both-pipelines-readable-rule-uuid> \
BUILDKITE_NONADMIN_RULE_HIDDEN_PIPELINE_UUID=<inaccessible-pipeline-uuid> \
BUILDKITE_NONADMIN_RULE_SOURCE_ONLY_UUID=<source-only-readable-rule-uuid> \
BUILDKITE_NONADMIN_RULE_TARGET_ONLY_UUID=<target-only-readable-rule-uuid> \
make testacc-nonadmin
```

The token needs the `graphql` and `write_pipelines` scopes. Its owner must be an
organization `MEMBER`, not an `ADMIN`, and a maintainer of the existing `everyone` team with
permission to create pipelines. Set `BUILDKITE_NONADMIN_TEAM_SLUG` to use another
existing team. The tests do not create teams or change memberships.

These tests create, read, update, import, and delete pipelines and schedules.
Pipelines are assigned to the team at creation using `default_team_id`, and
schedules remain disabled so they cannot trigger builds. The pipeline lifecycle
also checks pipeline and team data sources, including filtered pipeline reads
after an update and unchanged private visibility. It reuses its pipeline for
these checks instead of creating a separate data source fixture.

Read-only tests look up the token owner's organization membership and list
members without assuming list order. The organization data source must return
its identifiers even when the token cannot read restricted API settings. These
tests do not change organization settings or memberships.

Set `BUILDKITE_NONADMIN_RULE_UUID` for both acceptance targets to an existing Rule
between two distinct pipelines the member can read. Member Rules reads must be
enabled for the token owner. An administrator must provision this fixture; the
tests never create, update, or delete it. The shared test checks data source reads
by UUID and GraphQL ID, then resource import and its refresh in a temporary state
that is not used by acceptance cleanup. Missing or invisible fixtures fail with
an actionable error rather than skipping. These reads need only `graphql`, not
an additional REST scope. See the [Rules cookbook](https://buildkite.com/docs/apis/graphql/cookbooks/rules).

Provision a third private pipeline with no access for the member, including
through `everyone` or another team. Set its UUID in
`BUILDKITE_NONADMIN_RULE_HIDDEN_PIPELINE_UUID`. Provision two more persistent Rules:

- `BUILDKITE_NONADMIN_RULE_SOURCE_ONLY_UUID`: from the readable source pipeline to
  the inaccessible pipeline.
- `BUILDKITE_NONADMIN_RULE_TARGET_ONLY_UUID`: from the inaccessible pipeline to
  the readable target pipeline.

Pass all four Rule fixture variables to both acceptance targets. The admin lane
must read both restricted Rules by UUID and ID and verify their endpoints; the
member lane must receive the not-found-or-inaccessible diagnostic for each
lookup. The test independently checks all three pipelines' visibility and keeps
the both-pipelines-readable Rule as a positive control. This prevents unintended
pipeline access or disabled member Rules reads from making the negative checks
misleadingly pass. No extra token scopes are needed, and none of these fixtures
are mutated or deleted by the tests. Do not use the persistent read fixtures for
Rule update or delete denial tests.

CI sets all four fixture UUIDs in `.buildkite/pipeline.yml` for both acceptance
steps; Docker Compose passes them into the test container. The
[source](https://buildkite.com/buildkite-terraform-provider-test-org/terraform-provider-non-admin-rules-source),
[target](https://buildkite.com/buildkite-terraform-provider-test-org/terraform-provider-non-admin-rules-target),
and [inaccessible](https://buildkite.com/buildkite-terraform-provider-test-org/terraform-provider-non-admin-rules-hidden)
pipelines are persistent fixtures, not resources for acceptance cleanup to delete.

The basic pipeline and schedule fixtures use only GraphQL. A separate shared
pipeline lifecycle uses REST writes to create and rename a custom slug and update
repository provider settings, requiring `write_pipelines`. It disables traditional
GitHub build triggering, verifies that team access survives the update, and checks
provider settings after refresh and import. These reads use GraphQL, so
`read_pipelines` is not needed. Independently configured GitHub Actions triggers
are unaffected by `trigger_mode = "none"`. GitHub webhook controls remain outside
this coverage. Add token scopes only when new coverage needs them; do not broaden
the token to run the admin suite unchanged.

Tests named `TestAccNonAdmin…` also run under the admin token with `make testacc`.
Use the same test implementation for operations supported by both credentials,
and consolidate equivalent admin scenarios rather than keeping duplicate tests.
The dedicated target checks the token owner's organization role and fails if its
test selector matches nothing. CI loads the non-admin token from
`/pipelines/buildkite/terraform-provider-buildkite-main/buildkite_api_token_nonadmin`
into `BUILDKITE_API_TOKEN` in a separate acceptance step. The admin and non-admin
steps share a concurrency group, produce per-job JUnit and JSON reports, and use
separate annotation contexts.

#### Cache Registry acceptance tests

Cache Registry live tests also require `BUILDKITE_CACHE_REGISTRIES_ACCEPTANCE=1`. Without it, they skip with a message that live resources are untested. The offline Terraform `UnitTest` lifecycle tests still run with `make test`.

To run both live tests against a local Buildkite instance with Cache Registries enabled and a seeded API token:

```bash
TF_ACC=1 BUILDKITE_CACHE_REGISTRIES_ACCEPTANCE=1 \
BUILDKITE_ORGANIZATION_SLUG=buildkite BUILDKITE_API_TOKEN=bkua_development_api_token \
BUILDKITE_GRAPHQL_URL=http://graphql.buildkite.localhost:3100/v1 \
BUILDKITE_REST_URL=http://api.buildkite.localhost:3100/v2 \
go test ./buildkite -run '^TestAccBuildkiteClusterCacheRegistry(Resource|ParentDeletion)$' -count=1 -v
```

The tests create disposable clusters and registries, exercise their lifecycle, and delete them. Once opted in, missing credentials, API failures, or unavailable Cache Registries fail the tests rather than skip them. Leave custom endpoint variables unset to use the default production APIs; an explicitly empty URL is not a default.

CI must opt in with `BUILDKITE_CACHE_REGISTRIES_ACCEPTANCE=1` only after its test organization has Cache Registries enabled and its token has organization membership and cluster management access. `TF_ACC=1` is still required (`make testacc` sets it). The Docker Compose test service forwards the opt-in and custom GraphQL/REST URLs when set.

#### Organization allowlist entries acceptance test

The tuple allowlist lifecycle uses the same Terraform steps against the offline organization test server (`make test`) and the live API. Live tuple testing is opt-in until the tuple REST endpoint is deployed:

```bash
TF_ACC=1 BUILDKITE_API_IP_ENTRIES_ACCEPTANCE=1 \
BUILDKITE_ORGANIZATION_SLUG=<disposable-org> BUILDKITE_API_TOKEN=<token> \
go test ./buildkite -run '^TestAccBuildkiteOrganizationIPEntries$' -count=1 -v
```

Set `BUILDKITE_REST_URL` and `BUILDKITE_GRAPHQL_URL` to target a development Buildkite instance with both backend PRs applied. [Backend preparation #37881](https://github.com/buildkite/buildkite/pull/37881) must deploy fleet-wide before enabling tuple writes from [REST tuples #37891](https://github.com/buildkite/buildkite/pull/37891). The tuple REST API must be available before releasing or using the provider's `allowed_api_ip_entries` functionality. The REST PR also requires human acceptance of rejecting writes containing both allowlist fields, including unmodified full GET responses.

Use a disposable organization with the allowlist feature and an administrator token with organization settings read/write scopes. The test replaces the entire allowlist, switches representations, imports, clears on attribute removal, and destroys. It includes `0.0.0.0/0` to retain API access. Without the opt-in, it explicitly skips; offline tests are not evidence that the live backend is deployed or accepted.

### Code Quality

Before committing, ensure your code passes these checks:

**Linting:**

```bash
golangci-lint run
```

**Formatting:**

```bash
gofmt -s -w .
```

**Vet:**

```bash
make vet
```

**Documentation:**

```bash
make docs
```

If you've installed Lefthook and your hooks (`lefthook install`), these checks will run automatically on commit.

## Manual Testing

The repo contains a `tf-proj/` directory that can be used to quickly test a compiled version of the provider from the current branch.

1. Update `tf-proj/main.tf` to use the resource or property you're developing
2. Compile the provider with `make build`
3. Add a `.terraformrc` configuration file to override the provider binary (see below)
4. Run `terraform plan` in the tf-proj directory:

```bash
BUILDKITE_API_TOKEN=<api-token> BUILDKITE_ORGANIZATION_SLUG=<org-slug> terraform plan
```

### Overriding the Provider for Local Development

Add a provider override to your `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "buildkite/buildkite" = "/path/to/terraform-provider-buildkite/"
  }

  direct {}
}
```

See the [Terraform documentation](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers) for more details.

## Development Guidelines

### API Usage

Buildkite has two APIs: REST and GraphQL. **New resources should use the GraphQL API where possible**, but can fall back to the REST API for resources or properties not yet supported by GraphQL.

### Bounding API calls

Every API call should run under a deadline derived from its operation's configured timeout.

A `Read` (and an import or plan-time lookup, which use the read timeout) derives one context and passes it to every call it makes, including `retry.RetryContext` and paging loops, so the whole read shares one budget:

```go
timeout, diags := r.client.readTimeout(ctx)
resp.Diagnostics.Append(diags...)
if resp.Diagnostics.HasError() {
    return
}

requestCtx, cancel := context.WithTimeout(ctx, timeout)
defer cancel()
```

A `Create`, `Update` or `Delete` derives a context from `mutationContext` for each step that changes something, meaning each `retry.RetryContext` block or each call made outside one, and passes the parent `ctx` to `retry.RetryContext`:

```go
requestCtx, cancel := mutationContext(ctx, timeout)
defer cancel()

err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
    _, err := archivePipeline(requestCtx, r.client.genqlient, id)
    return retryContextError(err)
})
```

`mutationContext` adds 30 seconds to the deadline, matching how long `retry.RetryContext` waits for an attempt still in flight at its timeout. So a mutation that lands just after the timeout is still recorded in state instead of being cancelled, which would leave it applied in Buildkite with no state. A new context per step stops a slow step from using up the time the steps after it need, once the first has already changed something. A helper that takes the timeout, such as `updatePipelineSlug`, derives its own context and takes the parent. A step that only looks something up, such as the repository check before a pipeline webhook is created, changes nothing that needs recording, so it uses a plain `context.WithTimeout` as a read does.

Keep the bare `ctx` for framework calls such as `resp.State.Set`, so state can still be recorded after the budget is spent. A request made without a deadline falls back to the read timeout, which stops it running the retry schedule out, but that fallback applies per request and uses the read timeout whatever the operation.

### Generating GraphQL Code

If you're working with GraphQL queries:

1. Generate the schema:
   ```bash
   make schema
   ```

2. Generate the GraphQL code:
   ```bash
   make generate
   ```

### Commit Messages

- Aim for "atomic commits" - each commit should be a single logical change
- Use clear, descriptive commit messages
- Reference relevant issues in your commits

### Code Style

- Follow standard Go conventions
- Run `go fmt` on your code
- Ensure all tests pass before submitting a PR

## Continuous Integration

There is a continuous integration pipeline on Buildkite:
https://buildkite.com/buildkite/terraform-provider-buildkite-main

The CI pipeline runs:
- Linting (`golangci-lint`)
- Code vetting (`go vet`)
- Unit tests
- Acceptance tests
- Documentation generation
- Build verification
- Security scanning

## Release Process

Pushing a new version tag to GitHub (or creating a new release on github.com) will trigger a new build in the release pipeline. That pipeline will:

1. Compile the appropriate binaries
2. Sign them
3. Attach them to a draft release in https://github.com/buildkite/terraform-provider-buildkite

A maintainer will edit the draft to add the relevant changes and click publish.

The [Terraform Registry](https://registry.terraform.io) will detect the new release on GitHub and update their own listings.

## Reporting Issues

If you encounter any issues or have suggestions for improvements, please open an issue on the GitHub repository with detailed information about:

- What you were trying to do
- What happened
- What you expected to happen
- Steps to reproduce the issue
- Your environment (OS, Go version, Terraform version)

## Getting API Tokens

To develop and test the provider, you'll need an API token with permissions suitable for the actions you want to perform. It's recommended that you create a dedicated organization for Terraform plugin development, given the broad range of permissions required.

You can generate one here: https://buildkite.com/user/api-access-tokens/new?description=terraform

## Contact

If the review is taking too long, feel free to ping the team through:
- GitHub comments
- Email: support@buildkite.com

Happy contributing!
