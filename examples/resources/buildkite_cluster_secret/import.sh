# Import a cluster secret using {cluster_id}/{secret_id}
#
# You can find the cluster_id under cluster settings in the UI
# and find the secret_id from the secrets list using the
# REST API response from:
# GET /v2/organizations/{org_slug}/clusters/{cluster_id}/secrets
terraform import buildkite_cluster_secret.example 01234567-89ab-cdef-0123-456789abcdef/fedcba98-7654-3210-fedc-ba9876543210
