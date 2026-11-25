resource "buildkite_deployment_artifact" "api" {
  name = "payments-api"
}

resource "buildkite_component" "api" {
  name          = "payments-api"
  artifact_uuid = buildkite_deployment_artifact.api.uuid
}

resource "buildkite_target" "production" {
  name = "production-au"
}

resource "buildkite_component_instance" "api" {
  target_uuid    = buildkite_target.production.uuid
  component_uuid = buildkite_component.api.uuid
  # Omit name to use the Component's name on create.
}

# Multiple differently named Instances can register the same Component on a Target.
resource "buildkite_component_instance" "worker" {
  name           = "payments-worker"
  target_uuid    = buildkite_target.production.uuid
  component_uuid = buildkite_component.api.uuid
}

# Changing a name renames in place. Changing a binding requires replacement.
# No versions are published and no deployment state is managed here.
