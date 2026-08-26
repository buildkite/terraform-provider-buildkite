# import a registry resource using the registry slug
#
# you can find the slug from the Buildkite web UI in the registry's URL:
# https://buildkite.com/organizations/{org}/packages/registries/{slug}
#
# or by listing all registries via the REST API:
# GET https://api.buildkite.com/v2/packages/organizations/{org}/registries
terraform import buildkite_registry.example example
