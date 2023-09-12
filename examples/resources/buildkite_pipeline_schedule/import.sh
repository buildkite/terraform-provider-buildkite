# import a pipeline schedule resource using the schedules GraphQL ID
#
# you can use this query to find the schedule:
# query getPipelineScheduleId {
#   organization(slug: "ORGANIZATION_SLUG") {
#         pipelines(first: 5, search: "PIPELINE_SEARCH_TERM") {
#       edges{
#         node{
#           name
#           schedules{
#             edges{
#               node{
#                 id
#               }
#             }
#           }
#         }
#       }
#     }
#   }
# }
terraform import buildkite_pipeline_schedule.test UGlwZWxpgm5Tf2hhZHVsZ35tLWRk4DdmN7c4LTA5M2ItNDM9YS0gMWE0LTAwZDUgYTAxYvRf49==
