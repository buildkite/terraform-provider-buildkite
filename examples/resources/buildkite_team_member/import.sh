# import a team member resource using the GraphQL ID
#
# you can use this query to find the ID:
# query getTeamMemberId {
#   organization(slug: "ORGANIZATION_SLUG") {
#     teams(first: 2, search: "TEAM_SEARCH_TERM") {
#       edges {
#         node {
#           members(first: 2, search: "TEAM_MEMBER_SEARCH_TERM") {
#             edges {
#               node {
#                 id
#               }
#             }
#           }
#         }
#       }
#     }
#   }
# }
terraform import buildkite_team_member.a_smith VGVhbU1lbWJlci0tLTVlZDEyMmY2LTM2NjQtNDI1MS04YzMwLTc4NjRiMDdiZDQ4Zg==
