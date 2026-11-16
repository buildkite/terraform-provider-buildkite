# Import an agent image using {cluster_id}/{image_id} or {cluster_id}/{image_name}.
# Names match exactly and are case-sensitive. An ID match takes precedence over a
# name match. Terraform stores the resolved image ID in state for either form.
#
# You can find the cluster_id under cluster settings in the UI
# and the image_id from the REST API response from:
# GET /v2/organizations/{org_slug}/clusters/{cluster_id}/agent-images
terraform import buildkite_cluster_agent_image.ruby 01234567-89ab-cdef-0123-456789abcdef/snb5nqcik8ge2

# Alternatively, import the same image by name:
terraform import buildkite_cluster_agent_image.ruby '01234567-89ab-cdef-0123-456789abcdef/ruby'
