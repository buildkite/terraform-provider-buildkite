# import a test suite team resource using the GraphQL ID
#
# you can use this query to find the ID:
# query getTeamSuiteIds {
#   organization(slug: "ORGANIZATION_SLUG") {
#     suites(first: 1, search:"SUITE_SEARCH_TERM") {
#       edges {
#         node {
#           id
#           name
#           teams(first: 10){
#             edges {
#               node {
#                 id
#                 accessLevel
#                 team{
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
terraform import buildkite_test_suite_team.main_everyone VGVhbvDf4eRef20tMzIxMGEfYTctNzEF5g00M8f5s6E2YjYtODNlOGNlZgD6HcBi
