# minimal portal
resource "buildkite_portal" "viewer" {
  slug  = "viewer-info"
  name  = "Viewer Information"
  query = "{ viewer { user { name email } } }"
}

# portal with all optional fields
resource "buildkite_portal" "restricted" {
  slug                 = "restricted-portal"
  name                 = "Restricted Portal"
  description          = "Portal with IP restrictions and custom settings"
  query                = "{ viewer { user { name email avatar { url } } } }"
  user_invokable       = true
  allowed_ip_addresses = "192.168.1.0/24 10.0.0.0/8"
}

# portal with complex GraphQL query
resource "buildkite_portal" "pipeline_stats" {
  slug        = "pipeline-statistics"
  name        = "Pipeline Statistics"
  description = "Returns statistics for organization pipelines"
  query       = <<-EOT
    {
      organization(slug: "my-org") {
        pipelines(first: 10) {
          edges {
            node {
              name
              slug
              builds(first: 5) {
                edges {
                  node {
                    number
                    state
                    createdAt
                  }
                }
              }
            }
          }
        }
      }
    }
  EOT

  user_invokable = false
}
