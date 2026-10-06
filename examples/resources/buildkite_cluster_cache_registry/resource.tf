resource "buildkite_cluster" "primary" {
  name        = "Primary cluster"
  description = "Runs the monolith build and deploy"
}

resource "buildkite_cluster_cache_registry" "primary" {
  cluster_id  = buildkite_cluster.primary.id
  name        = "Primary cache"
  description = "Shared build cache"
  emoji       = ":package:"
  color       = "#BADA55"

  policy = jsonencode({
    save = {
      scopes = {
        branch = true
      }
    }
    restore = {
      scopes = [{ branch = "main" }]
    }
    rules = [
      { effect = "allow", action = "save" },
      { effect = "allow", action = "restore" },
    ]
  })
}
