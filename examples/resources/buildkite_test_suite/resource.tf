# create a test suite for the main repository
resource "buildkite_test_suite" "main" {
  name           = "main"
  default_branch = "main"
  team_owner_id  = "VGVhbvDf4eRef20tMzIxMGEfYTctNzEF5g00M8f5s6E2YjYtODNlOGNlZgD6HcBi"
}
