# create a test suite for the main repository
resource "buildkite_test_suite" "main" {
  name           = "main"
  default_branch = "main"
  emoji          = ":buildkite:"
  team_owner_id  = "VGVhbvDf4eRef20tMzIxMGEfYTctNzEF5g00M8f5s6E2YjYtODNlOGNlZgD6HcBi"
}

# enable test state management and allow muting tests for quarantine
# requires a plan that includes test state management; automatic change-state
# workflow actions additionally require Enterprise
resource "buildkite_test_suite" "with_state_management" {
  name           = "with state management"
  default_branch = "main"
  team_owner_id  = "VGVhbvDf4eRef20tMzIxMGEfYTctNzEF5g00M8f5s6E2YjYtODNlOGNlZgD6HcBi"

  test_state_management = true
  allowed_test_states   = ["muted"]

  # Disabling permanently resets muted/skipped tests to enabled. Re-enabling
  # does not restore their previous states.
  # Removing these attributes from config leaves the remote settings unmanaged.
}

# create a test suite without the default flaky-detection workflow or flaky label
# this option only takes effect when creating the suite
resource "buildkite_test_suite" "without_flaky_detection" {
  name            = "without flaky detection"
  default_branch  = "main"
  team_owner_id   = "VGVhbvDf4eRef20tMzIxMGEfYTctNzEF5g00M8f5s6E2YjYtODNlOGNlZgD6HcBi"
  flaky_detection = false
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
