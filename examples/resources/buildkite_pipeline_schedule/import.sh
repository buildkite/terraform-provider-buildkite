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

# or using the pipeline slug and the schedule UUID (slugs can change; the GraphQL ID is the stable identifier)
terraform import buildkite_pipeline_schedule.test my-pipeline/dd837f77-8093-4b3d-a1a4-00d5a01af4d5
