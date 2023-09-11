# import a pipeline team resource using the GraphQL ID
#
# you can use this query to find the ID:
# query getPipelineTeamId {
#   pipeline(slug: "ORGANIZATION_SLUG/PIPELINE_SLUG") {
#     teams(first: 5, search: "PIPELINE_SEARCH_TERM") {
#       edges{
#         node{
#           id
#         }
#       }
#     }
#   }
# }
terraform import buildkite_pipeline_team.guests VGVhbS0tLWU1YjQyMDQyLTUzN2QtNDZjNi04MjY0LTliZjFkMzkyYjZkNQ==
