data "buildkite_organization" "current" {}

data "buildkite_pipeline" "release" {
  slug = "release"
}

# Let jobs in builds of the deploy pipeline's main branch create builds of the release pipeline.
# A job requests an OIDC token with:
#   buildkite-agent oidc request-token --audience https://api.buildkite.com/v2/organizations/<organization UUID>
resource "buildkite_workload_identity" "deploy" {
  name        = "Deploy pipeline"
  description = "Creates release builds from deploy jobs"

  oidc_issuer   = "https://agent.buildkite.com"
  oidc_audience = "https://api.buildkite.com/v2/organizations/${data.buildkite_organization.current.uuid}"
  oidc_subject  = "organization:acme-inc:pipeline:deploy:ref:refs/heads/main:commit:*:step:*"

  # Tokens from Buildkite jobs must be limited to your organization
  oidc_claims = jsonencode({
    organization_id = data.buildkite_organization.current.uuid
  })

  grantable_token_scopes = ["read_builds", "write_builds"]
  default_token_scopes   = ["read_builds", "write_builds"]
  max_token_ttl_seconds  = 600

  resource_grants = [{
    type   = "pipeline"
    access = "selected"
    ids    = [data.buildkite_pipeline.release.uuid]
  }]
}

# Let the report step of the release pipeline read builds and their logs from every pipeline,
# in builds of the main branch or a release branch
resource "buildkite_workload_identity" "release_reports" {
  name        = "Release reports"
  description = "Reads build results for release reports"

  oidc_issuer   = "https://agent.buildkite.com"
  oidc_audience = "https://api.buildkite.com/v2/organizations/${data.buildkite_organization.current.uuid}"
  oidc_subject  = "organization:acme-inc:pipeline:release:ref:refs/heads/*:commit:*:step:report"

  oidc_claims = jsonencode({
    organization_id = data.buildkite_organization.current.uuid
  })
  oidc_claim_conditions = "claims.build_branch == \"main\" || claims.build_branch.startsWith(\"release/\")"

  grantable_token_scopes = ["read_builds", "read_build_logs"]

  resource_grants = [{
    type   = "pipeline"
    access = "all"
  }]
}
