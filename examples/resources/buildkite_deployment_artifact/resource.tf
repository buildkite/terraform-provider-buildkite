resource "buildkite_deployment_artifact" "api" {
  name = "payments-api"
}

# Destroy archives this registration and reserves its name.
# Set allow_unarchive_on_create = true only to restore an archived registration.
