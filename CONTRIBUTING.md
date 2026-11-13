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

#### Cleaning up acceptance resources

Acceptance tests use names such as `tfacc-5319-80c886d-abcdefghijkl`: the `tfacc-`
marker, Buildkite build number, first seven commit characters, and a random suffix.
Local runs use `tfacc-local-<random>`. Use `testAccResourceName()` for temporary
names, including updated names, and `testAccResourceKey()` for secret keys, which
use uppercase letters and underscores instead of hyphens. Keep random secret
values separate from resource identities. Resources without names belong to
marked temporary parents; tokens and notification services use marked descriptions.

The CI `sweepers` step runs on the `oss-deploy` queue before both acceptance jobs,
using the existing IAM role's source-IP restrictions. All three steps share the
`terraform-provider-acceptance-tests` concurrency group.
Sweep failures are reported as soft failures and do not fail the build. Both
acceptance jobs wait for the sweep to finish but allow dependency failures, so a
cleanup failure does not prevent acceptance testing.

To invoke the [HashiCorp sweeper](https://developer.hashicorp.com/terraform/plugin/testing/acceptance-tests/sweepers)
manually, export `BUILDKITE_API_TOKEN` for a test organization, then run:

```bash
BUILDKITE_ORGANIZATION_SLUG=<test-org> make sweep
```

This command deletes real resources. Run it only in a dedicated test organization,
with an admin token that has `graphql` and the REST read/write scopes for pipelines,
clusters, teams, Rules, portals, pipeline templates, notification services, suites, secret
details, and secrets, plus `delete_registries`. GraphQL supplies suite and package
registry creation timestamps because their REST responses omit them. The sweeper
reads secret metadata only, never secret values.

The sweep selects marked resources strictly older than 24 hours using API creation
timestamps. It removes Rules, pipelines, portals, templates, suites, package
registries, temporary cluster children, clusters, notification services, teams,
and organization-wide agent tokens. Supported repository webhooks are removed before
their pipelines; the organization's GitHub App needs permission to read and delete
them. Rules without descriptions can be identified through marked pipeline
endpoints. Pipelines and their clusters are retained until dependent Rules are
old enough to sweep, preserving that ownership evidence. Cluster deletion also cleans up
its remaining queues, tokens, cache registries, and hosted tenant, including agent
images. Clusters are retained while any discovered child has not reached the age
cutoff or has no creation timestamp. They are also retained while any secret is
preserved, including unmarked secrets, because cluster deletion does not remove
secrets and their API requires a surviving cluster. Marked queues, tokens, secrets,
and non-default cache registries can be removed from persistent clusters without
deleting those clusters.

Persistent non-admin fixtures are preserved. Unmarked legacy leftovers, including
resources created before this naming convention, require manual review and cleanup.
Webhook discovery uses the pipeline's current repository; hooks left in a previous
repository after a repository/provider change need manual cleanup.
Objects without creation timestamps are not individually swept. In particular,
organization banners and changes to organization settings need manual recovery;
agent images are cleaned up through their temporary parent cluster. Default cache
registries in persistent clusters are preserved because the API prohibits deleting
them. Organization-wide token discovery fails explicitly if its non-paginated
GraphQL list is truncated.

#### Non-admin acceptance tests

Run the shared pipeline, pipeline schedule, pipeline-team, queue, cluster agent-token,
test suite, and suite-team lifecycles and read-only data source tests with a non-admin token:

```bash
BUILDKITE_ORGANIZATION_SLUG=<test-org> BUILDKITE_API_TOKEN=<non-admin-token> \
make testacc-nonadmin
```

Use `buildkite-terraform-provider-test-org` as the test organization. The persistent
fixture names and IDs are constants in `buildkite/provider_test.go`, shared by both
acceptance targets. They are not environment-variable overrides. These scenarios
require the fixtures in that organization; running them in another organization
requires updating the constants. Do not delete the persistent fixtures or change
their required access grants.

The token needs `graphql`, `write_pipelines`, `read_clusters`, `read_suites`,
`write_suites`, `read_secrets_details`, and `write_secrets` scopes. Its owner must be an
organization `MEMBER`, not an `ADMIN`, and a maintainer of the existing `everyone` team with
permission to create pipelines. Set `BUILDKITE_NONADMIN_TEAM_SLUG` to use another
existing team. The tests do not create teams or change memberships.

These tests create, read, update, import, and delete pipelines and schedules.
Pipelines are assigned to the team at creation using `default_team_id`, and
schedules remain disabled so they cannot trigger builds. The pipeline lifecycle
also checks pipeline and team data sources, including filtered pipeline reads
after an update and unchanged private visibility. It reuses its pipeline for
these checks instead of creating a separate data source fixture.

The shared pipeline-template lifecycle reads the persistent available template
identified by `testAccFixtureTemplateName`, `testAccFixtureTemplateUUID`, and
`testAccFixtureTemplateID`, independently by name and GraphQL ID. Its configuration
must remain `steps: []`. The test creates a temporary private pipeline with that
template, refreshes, imports by pipeline slug, removes the assignment in place
with explicit nonempty steps, reassigns the template, and deletes the pipeline.
The organization must permit editable pipeline steps so the member can remove
the template assignment and configure explicit steps.
Checks require matching remote assignment and steps, stable pipeline identity,
the original team's unchanged Full Access, and remote absence after deletion.
The template and team remain data sources; neither is managed or deleted.
These operations use GraphQL and require no additional REST scopes. Equivalent
basic template data-source reads are consolidated here; nonempty-template
assignment, template-update propagation, explicit/implicit steps-mode transitions,
and invalid data-source lookup cases remain separate.

`testAccFixturePipelineTeamSlug` identifies the second persistent team, distinct
from the pipeline's original Full Access team.
The member token owner must also maintain this team. The shared pipeline-team
test creates a temporary pipeline, adds the second team with read-only access,
updates it to Full Access, refreshes, and imports the association using both its
GraphQL ID and `pipeline-slug/team-slug`. It then removes the association and
checks that the original team's Full Access remains intact before deleting the
temporary pipeline. Both persistent teams and their memberships remain unchanged.
These operations use GraphQL and need no additional REST scopes.

`testAccFixtureClusterName` and `testAccFixtureClusterUUID` identify the persistent
cluster. The second team above must remain a cluster maintainer. The shared
cluster test reads and refreshes the cluster through its data source,
checks its name, UUID, and GraphQL ID, and requires the known team in the REST
maintainer list without assuming list order. A forbidden maintainer lookup must
not pass as an empty list. This read requires `read_clusters` and cluster-management
permission inherited through the team; it does not need `write_clusters`. The
test never manages or deletes the cluster or its maintainer assignments.

The shared self-hosted queue lifecycle uses the same cluster fixture. It creates a
temporary queue, updates its description in place, pauses dispatch, refreshes,
imports using both `id,cluster_uuid` and `cluster_uuid/key`, resumes dispatch, and
deletes the queue. It checks remote settings and deletion, stable queue identity,
and the cluster's unchanged default queue. The test does not manage the persistent
cluster, maintainers, or default-queue assignment. Queue mutations use GraphQL;
REST reads during refresh and import require the existing `read_clusters` scope.
The test leaves retry affinity unconfigured at its default. Changing affinity
would require `write_clusters` and remains in the admin-only coverage.

The shared cluster agent-token lifecycle also uses this cluster fixture. It
creates a temporary token with restricted IP addresses and a finite expiry,
updates its description and IP restrictions in place, refreshes, and revokes it.
Checks require stable token identity and secret, matching remote settings, and
remote absence after revocation. The cluster remains a data source and is never
managed or deleted. Token operations use GraphQL; the cluster data source uses
the existing `read_clusters` scope for maintainer reads. The token resource does
not support import. Distinct expiry replacement cases remain in the admin suite.

The shared cluster-secret lifecycle creates a temporary secret on the same
maintained cluster, updates its description and policy, rotates a generated dummy
value, refreshes, imports using `cluster_uuid/secret_uuid`, and deletes it. It
checks stable identity and creation time, remote metadata, state-value retention,
and remote absence after deletion. Import verification excludes `value` because
the API does not return secret values; imported state must not contain a value.
The test does not retrieve a secret through an agent or prove its encrypted value
by reading it back. Secret metadata reads require `read_secrets_details`; creation,
both update operations, and deletion require `write_secrets`. The cluster remains
a data source. Distinct write-only, value-to-write-only migration, policy, and
validation cases remain separate.

The shared test suite lifecycle uses `testAccFixturePipelineTeamSlug` as the
owner of a temporary suite. The member must maintain that team, and the
organization must allow members to delete test suites. The test creates the
suite, reads its data source, updates its default branch and application name in
place, refreshes, imports by GraphQL ID, and deletes it. Checks require stable
identity and API token, the owner's unchanged Manage & Read access, and remote
absence after deletion. The persistent team and its memberships are never
managed. REST operations require `read_suites` and `write_suites`; ownership
reads use GraphQL. Distinct optional-setting and owner-change scenarios remain
in the admin suite.

The shared suite-team lifecycle creates another temporary suite with the same
owner and adds the `BUILDKITE_NONADMIN_TEAM_SLUG` team (default `everyone`) with
Read Only access. These teams must be distinct. The test updates the secondary
association to Manage & Read, refreshes, imports by GraphQL ID, and removes it
before suite cleanup. Checks require a stable association identity, remote
absence after removal, and the original owner's unchanged Manage & Read access.
Both teams remain data sources. Association operations use GraphQL and need no
additional REST scopes. Team replacement and out-of-band removal scenarios
remain in the admin suite.

Read-only tests look up the token owner's organization membership and list
members without assuming list order. The organization data source must return
its identifiers even when the token cannot read restricted API settings. These
tests do not change organization settings or memberships.

`testAccFixtureReadableRuleUUID` identifies the persistent Rule between two
distinct pipelines the member can read. Member Rules reads must be enabled for
the token owner. An administrator must provision this fixture; the tests never
create, update, or delete it. The shared test checks data source reads
by UUID and GraphQL ID, then resource import and its refresh in a temporary state
that is not used by acceptance cleanup. Missing or invisible fixtures fail with
an actionable error rather than skipping. These reads need only `graphql`, not
an additional REST scope. See the [Rules cookbook](https://buildkite.com/docs/apis/graphql/cookbooks/rules).

`testAccFixtureHiddenPipelineUUID` identifies a third private pipeline with no
access for the member, including through `everyone` or another team. Two more
persistent Rules exercise partial endpoint visibility:

- `testAccFixtureSourceOnlyRuleUUID`: from the readable source pipeline to
  the inaccessible pipeline.
- `testAccFixtureTargetOnlyRuleUUID`: from the inaccessible pipeline to
  the readable target pipeline.

The admin lane must read both restricted Rules by UUID and ID and verify their
endpoints; the member lane must receive the not-found-or-inaccessible diagnostic for each
lookup. The test independently checks all three pipelines' visibility and keeps
the both-pipelines-readable Rule as a positive control. This prevents unintended
pipeline access or disabled member Rules reads from making the negative checks
misleadingly pass. No extra token scopes are needed, and none of these fixtures
are mutated or deleted by the tests. Do not use the persistent read fixtures for
Rule update or delete denial tests.

The shared Rule creation test reuses the two readable pipelines. Their persistent
Rule must use `pipeline.artifacts_read.pipeline`, and there must be no
`pipeline.trigger_build.pipeline` Rule between the same source and target before
the test. The admin lane creates and deletes a temporary trigger Rule with a
unique fixture-only branch condition; the member lane must receive the explicit
API permission error. Rules management remains admin-only even when a member can
read both pipelines. Creating a Rule does not itself trigger a build.

Before the attempt and after Terraform cleanup, an endpoint-filtered Rules query
must include the persistent artifacts-read control and no trigger Rule. Member
Rules listing must also be enabled for the organization. These checks require
only `graphql`, no new persistent fixtures or REST scopes. If an unexpected Rule
remains, the test reports its ID for an administrator to inspect and remove;
it never falls back to admin credentials. Persistent read Rules remain untouched.

Rule update/delete denials run in `make testacc`, which additionally requires
`BUILDKITE_NONADMIN_API_TOKEN` containing the member token. `BUILDKITE_API_TOKEN`
remains the admin token. Terraform creates a disposable trigger Rule using the
readable fixture pipelines and verifies an admin update succeeds. Separate
member-configured Terraform states import that Rule and require explicit update
and delete permission errors. Data source reads then verify its identity,
description, conditions, and endpoints remain unchanged. Each member state
forgets its imported copy without deleting the Rule; the independent admin state
owns cleanup, including when a denial test fails. No member operation is retried
with admin credentials, and no persistent Rule is managed by this test.

This test requires Terraform 1.7 or newer for non-destructive `removed` blocks
(CI uses 1.16). It runs serially within the admin suite to avoid occupying the
shared trigger slot concurrently with the creation test. CI supplies the existing
member token to the admin step under the additional variable; the non-admin step
still receives only the member token. Neither token needs additional API scopes.

Registry permission tests also run in `make testacc` with both tokens. The member
token needs `read_registries`, `write_registries`, and `delete_registries`, and its
team must have permission to create and delete registries. The tests use
`testAccFixturePipelineTeamSlug` as an existing team data source; they never
manage the team or its memberships.

One case creates and reads a temporary private registry with the member token,
then forgets the member state without deleting the registry. Independent admin
Terraform cleanup imports and deletes that registry, including after failed
member steps. Another case creates a temporary registry with the admin token,
imports its existing team association, and explicitly grants Full Access using
Terraform. Admin and member checks then verify registry settings, data source
reads, updates, and imports by slug. The member refreshes and deletes the registry;
the independent admin state retains cleanup responsibility if a member step fails.
Checks require stable identity, unchanged private visibility and team Full Access,
and remote absence after deletion. No member operation is retried with admin
credentials. The non-admin-only job does not receive the admin token.

Registry-team permission tests use the same dual-credential admin job. Admin
Terraform creates a temporary registry and grants the existing owner team Full
Access. Admin and member states then separately exercise secondary-team creation,
Read Only → Read & Write → Full Access updates, GraphQL-ID import, refresh, and
removal on that registry. The secondary team is `BUILDKITE_NONADMIN_TEAM_SLUG`
(default `everyone`); it must differ from the owner, and the member must maintain
both teams. Both teams remain data sources. Checks require stable association
identity, correct endpoints and access, unchanged owner Full Access, and registry
survival after association removal. Independent admin Terraform state owns final
registry cleanup, including after member failures. Association mutations use
GraphQL; no additional API scopes or persistent fixtures are needed beyond the
registry prerequisites above. The distinct admin drift/recreation test remains.

The
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

A `Read` (and an import or plan-time lookup, which use the read timeout) derives one context and passes it to every call it makes, including retry helpers and paging loops, so the whole read shares one budget. Read retry helpers must reuse that context rather than create a new deadline for each retry block:

```go
timeout, diags := r.client.readTimeout(ctx)
resp.Diagnostics.Append(diags...)
if resp.Diagnostics.HasError() {
    return
}

requestCtx, cancel := context.WithTimeout(ctx, timeout)
defer cancel()
```

The initial cluster and pipeline GraphQL refreshes use `retryRead(requestCtx, fn)`. It waits synchronously for the request's error, so an SDK waiter cannot return a generic timeout before the request reports its captured API failure. The HTTP client owns transport and HTTP-status retries; `retryRead` retries only transient backend errors in GraphQL responses. If the read runs out of time, it retains the latest backend error alongside the deadline or cancellation error. It does not add a mutation grace period or reset the shared deadline.

A `Create`, `Update` or `Delete` gives each step that changes something its own deadline. A step that retries uses `retryMutation` in place of `retry.RetryContext`. `retryMutation` passes the parent `ctx` to `retry.RetryContext` and hands the function a context from `mutationContext`, so make every call in the block with the `ctx` the function receives:

```go
err := retryMutation(ctx, timeout, func(ctx context.Context) *retry.RetryError {
    _, err := archivePipeline(ctx, r.client.genqlient, id)
    return retryContextError(err)
})
```

A call made outside a retry block derives its own context from `mutationContext` and passes it to the call:

```go
requestCtx, cancel := mutationContext(ctx, timeout)
defer cancel()

_, err := setOrganization2FA(requestCtx, r.client.genqlient, org, enforce)
```

`mutationContext` adds 30 seconds to the deadline, matching how long `retry.RetryContext` waits for an attempt still in flight at its timeout. So a mutation that lands just after the timeout is still recorded in state instead of being cancelled, which would leave it applied in Buildkite with no state. A new context per step stops a slow step from using up the time the steps after it need, once the first has already changed something. A helper that takes the timeout, such as `updatePipelineSlug`, derives its own context and takes the parent. A step that only looks something up, such as the repository check before a pipeline webhook is created, changes nothing that needs recording, so it uses a plain `context.WithTimeout` as a read does. A lookup that pages, such as `suiteTeams`, shares that one context across its pages and their `retry.RetryContext` calls, the same as a read.

Keep the method's own `ctx`, outside any retry block, for framework calls such as `resp.State.Set`, so state can still be recorded after the budget is spent. A request made without a deadline falls back to the read timeout, which stops it running the retry schedule out, but that fallback applies per request and uses the read timeout whatever the operation.

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
