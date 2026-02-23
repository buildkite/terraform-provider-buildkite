# Import using cluster_id/secret_id format
terraform import buildkite_cluster_secret.example 01234567-89ab-cdef-0123-456789abcdef/fedcba98-7654-3210-fedc-ba9876543210
import {
  to = buildkite_cluster_secret.example
  id = "01234567-89ab-cdef-0123-456789abcdef/fedcba98-7654-3210-fedc-ba9876543210"
}
