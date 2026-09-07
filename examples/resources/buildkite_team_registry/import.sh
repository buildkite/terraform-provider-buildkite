# import a team registry resource using the GraphQL ID
#
# you can use this query to find the ID:
# query getTeamRegistryIds {
#   organization(slug: "ORGANIZATION_SLUG") {
#     registries(first: 1, search: "REGISTRY_SEARCH_TERM") {
#       edges {
#         node {
#           id
#           name
#           teams(first: 10) {
#             edges {
#               node {
#                 id
#                 accessLevel
#                 team {
#                   name
#                 }
#               }
#             }
#           }
#         }
#       }
#     }
#   }
# }
terraform import buildkite_team_registry.gems_everyone VGVhbvDf4eRef20tMzIxMGEfYTctNzEF5g00M8f5s6E2YjYtODNlOGNlZgD6HcBi
