# import a portal resource using the portal slug
#
# you can find the slug from the Buildkite web UI in the portal's URL:
# https://buildkite.com/organizations/{org}/portals/{slug}
#
# or by listing all portals via the REST API:
# GET https://api.buildkite.com/v2/organizations/{org}/portals
terraform import buildkite_portal.viewer viewer-info
