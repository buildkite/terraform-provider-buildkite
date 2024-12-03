# import an organization rule resource using the rules GraphQL ID
#
# You can use this query to find the first 50 organiation rules (adjust for less or more):
# query getOrganizationRules {
#   organization(slug: "ORGANIZATION_SLUG") {
#     rules(first: 50) {
#       edges{
#         node{
#           id
#           sourceType
#           targetType
#           action
#         }
#       }
#     }
#   }
# }
#
# Depending on the speciific source/target, you're also able to filter on the source/target information
# query getOrganizationRules {
#   organization(slug: "ORGANIZATION_SLUG") {
#     rules(first: 50) {
#       edges{
#         node{
#           id
#           sourceType
#           source {
#             ... on Pipeline{
#               uuid
#               name
#             }            
#           }
#           targetType
#           target {
#             ... on Pipeline{
#               uuid
#               name
#             }            
#           }
#           action
#         }
#       }
#     }
#   }
# }
terraform import buildkite_organization_rule.artifact_read UnVsZS0tLTAxOTE5NmU2LWNiNjctNzZiZi1iYzAyLTVhYzFiNzhhMWMyOA==
