# create a cluster
resource "buildkite_cluster" "primary" {
  name        = "Primary cluster"
  description = "Runs the monolith build and deploy"
  emoji       = "🚀"
  color       = "#bada55"
}

# create a basic secret for the cluster
resource "buildkite_cluster_secret" "api_key" {
  cluster_id  = buildkite_cluster.primary.id
  key         = "API_KEY"
  value       = var.api_key
  description = "API key for external service"
}

# create a secret with a policy that restricts access
resource "buildkite_cluster_secret" "deploy_key" {
  cluster_id  = buildkite_cluster.primary.id
  key         = "DEPLOY_SSH_KEY"
  value       = var.deploy_ssh_key
  description = "SSH key for deployments"
  policy      = <<-EOT
    - pipeline_slug: deploy-pipeline
      build_branch: main
  EOT
}

# use a pipeline that can access the secrets
resource "buildkite_pipeline" "monolith" {
  name       = "Monolith"
  repository = "https://github.com/..."
  cluster_id = buildkite_cluster.primary.id
}
