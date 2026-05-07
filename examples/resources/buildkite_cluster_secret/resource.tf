resource "buildkite_cluster_secret" "example" {
  cluster_id  = "01234567-89ab-cdef-0123-456789abcdef"
  key         = "DATABASE_PASSWORD"
  value       = "super-secret-password"
  description = "Production database password"
  policy      = <<-EOT
    - pipeline_slug: my-pipeline
      build_branch: main
  EOT
}

# Use Terraform write-only attributes to pass a secret value without storing it
# in Terraform plan or state artifacts. The version should be a non-secret marker
# that changes when the secret value changes.
variable "api_token" {
  type      = string
  sensitive = true
  ephemeral = true
}

variable "api_token_version" {
  type = string
}

resource "buildkite_cluster_secret" "write_only_example" {
  cluster_id       = "01234567-89ab-cdef-0123-456789abcdef"
  key              = "API_TOKEN"
  value_wo         = var.api_token
  value_wo_version = var.api_token_version
  description      = "API token"
}
