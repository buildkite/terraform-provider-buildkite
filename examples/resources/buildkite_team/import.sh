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
terraform import buildkite_team.everyone UGlwZWxpbmUtLS00MzVjYWQ1OC1lODFkLTQ1YWYtODYzNy1iMWNmODA3MDIzOGQ=
