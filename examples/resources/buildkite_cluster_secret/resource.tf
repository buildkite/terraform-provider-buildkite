resource "buildkite_cluster_secret" "example" {
  cluster_id  = "01234567-89ab-cdef-0123-456789abcdef"
  key         = "DATABASE_PASSWORD"
  value       = "super-secret-password"
  description = "Production database password"
  policy      = <<-EOT
    pipeline_slug: my-pipeline
    branch: main
  EOT
}