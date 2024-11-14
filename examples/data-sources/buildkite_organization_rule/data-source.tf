# Read an organization rule by its id
data "buildkite_organization_rule" "data_artifacts_read_dev_test" {
  id = buildkite_organization_rule.artifacts_read_dev_test.id
}

# Read an organization rule by its uuid
data "buildkite_organization_rule" "data_artifacts_read_test_dev" {
  uuid = buildkite_organization_rule.artifacts_read_test_dev.uuid
}