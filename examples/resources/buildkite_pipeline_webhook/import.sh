# import a pipeline webhook using the pipeline's GraphQL ID
#
# you can use this query to find the pipeline ID:
# query getPipelineId {
#   pipeline(slug: "ORGANIZATION_SLUG/PIPELINE_SLUG") {
#     id
#   }
# }
terraform import buildkite_pipeline_webhook.webhook UGlwZWxpbmUtLS0wMTkzYTNlOC1lYzc4LTQyNWEtYTM0Ny03YzRjNDczZDFlMGE=
