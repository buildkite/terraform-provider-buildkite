# import an organization banner resource using the banner's GraphQL ID
#
# you can use this query to find the banner's ID:
# query getOrganizationBannerId {
#   organization(slug: "ORGANIZATION_SLUG") {
#     banners(first: 1) {
#       edges {
#         node {
#           id
#         }
#       }
#     }
#   }
# }
terraform import buildkite_organization_banner.banner T3JnYW5pemF0aW9uQmFubmVyLS0tNjZlMmE5YzktM2IzMy00OGE5LTk1NjItMzY2YzMwNzYzN2Uz
