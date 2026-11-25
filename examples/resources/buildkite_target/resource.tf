resource "buildkite_target" "production" {
  name = "production-au"
  labels = {
    environment = "production"
    region      = "au"
  }
}

# The entire label map is managed: removing a key removes it remotely.
# Destroy archives the Target and its active Instances.
# Restoring a Target does not restore its Instances.
