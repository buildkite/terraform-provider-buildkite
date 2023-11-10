# import a pipeline template resource using the templates GraphQL ID
#
# You can use this query to find the first 50 templates (adjust for less or more):
# query getPipelineTemplateIds {
#   organization(slug: "ORGANIZATION_SLUG") {
#     pipelineTemplates(first: 50) {
#       edges{
#         node{
#           id
#           name
#         }
#       }
#     }
#   }
# }
terraform import buildkite_pipeline_template.template UGlwZWxpbmVUZW1wbGF0ZS0tLWU0YWQ3YjdjLTljZDYtNGM0MS1hYWE0LTY2ZmI3ODY0MTMwNw==
