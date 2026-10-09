# Import an agent image using {cluster_id}/{image_id}
#
# You can find the cluster_id under cluster settings in the UI
# and the image_id from the REST API response from:
# GET /v2/organizations/{org_slug}/clusters/{cluster_id}/agent-images
terraform import buildkite_hosted_agent_image.ruby 01234567-89ab-cdef-0123-456789abcdef/snb5nqcik8ge2
