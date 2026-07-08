# create a test suite for the main repository
resource "buildkite_test_suite" "main" {
  name           = "main"
  default_branch = "main"
  emoji          = ":buildkite:"
  team_owner_id  = "VGVhbvDf4eRef20tMzIxMGEfYTctNzEF5g00M8f5s6E2YjYtODNlOGNlZgD6HcBi"
}

# create a test suite with an OIDC policy allowing a pipeline to upload
# test results without a suite API token
resource "buildkite_test_suite" "with_oidc_policy" {
  name           = "with OIDC policy"
  default_branch = "main"
  team_owner_id  = "VGVhbvDf4eRef20tMzIxMGEfYTctNzEF5g00M8f5s6E2YjYtODNlOGNlZgD6HcBi"

  oidc_policy = <<-EOT
  - iss: https://agent.buildkite.com
    claims:
      organization_slug: my-org
      pipeline_slug: my-pipeline
    scopes:
      - read_suites
      - write_uploads
  EOT
}
