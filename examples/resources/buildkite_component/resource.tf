resource "buildkite_deployment_artifact" "api" {
  name = "payments-api"
}

# A Component belongs to the organization, not to a Target.
resource "buildkite_component" "api" {
  name          = "payments-api"
  artifact_uuid = buildkite_deployment_artifact.api.uuid
}

# Changing artifact_uuid requires replacement. Destroy reserves the old name,
# so use a different name for a Component with a different Artifact binding.
