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
terraform import buildkite_pipeline_team.guests VGVhbVBpcGVsaW5lLS0tMmQ5ZmRjYjctMjJjYS00ZDU3LTkwMWMtYmI3NzY1MmM5ZTk2
