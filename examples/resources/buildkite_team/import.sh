# import a team resource using the GraphQL ID
#
# you can use this query to find the ID:
# query getTeamId {
#   organization(slug: "ORGANIZATION_SLUG") {
#     teams(first: 1, search: "TEAM_SEARCH_TERM") {
#       edges {
#         node {
#           id
#           name
#         }
#       }
#     }
#   }
# }
terraform import buildkite_team.everyone VGVhbS0tLTdmOTdlZDZhLTQ4MjQtNDM2Yi04NWE0LTNlZDQ0YWRjY2IxMg==
