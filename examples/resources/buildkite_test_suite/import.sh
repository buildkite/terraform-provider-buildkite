# import a test suite resource using the GraphQL ID
#
# you can use this query to find the ID:
# query getSuiteIds {
#   organization(slug: "ORGANIZATION_SLUG") {
#     suites(first: 1, search:"SUITE_SEARCH_TERM") {
#       edges {
#         node {
#           id
#           name
#         }
#       }
#     }
#   }
# }
terraform import buildkite_test_suite.acceptance VGVhbvDf4eRef20tMzIxMGEfYTctNzEF5g00M8f5s6E2YjYtODNlOGNlZgD6HcBi
