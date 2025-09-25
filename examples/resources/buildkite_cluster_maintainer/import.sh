# Import a cluster maintainer using {cluster_id}/{permission_id}
#
# You can find the cluster_id (cluster UUID) and the permission_id
# from the maintainers list using the cluster data source or REST
# API response from:
# GET /v2/organizations/{org_slug}/clusters/{cluster_id}/maintainers
terraform import buildkite_cluster_maintainer.user_maintainer 01234567-89ab-cdef-0123-456789abcdef/977b68d3-f8fe-4784-8d43-5bc857e10541
