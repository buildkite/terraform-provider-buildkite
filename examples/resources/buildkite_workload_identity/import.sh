# Import a workload identity using its UUID
#
# You can find the UUID from the REST API response from:
# GET /v2/organizations/{org_slug}/workload-identities
terraform import buildkite_workload_identity.deploy 01a1b2c3-4d5e-7f60-8a1b-2c3d4e5f6a7b
