# import a pipeline resource using the pipelines GraphQL ID
# GraphQL ID for a pipeline can be found on its settings page
terraform import buildkite_pipeline.pipeline UGlwZWxpbmUtLS00MzVjYWQ1OC1lODFkLTQ1YWYtODYzNy1iMWNmODA3MDIzOGQ=

# or using its slug (slugs can change; the GraphQL ID is the stable identifier)
terraform import buildkite_pipeline.pipeline my-pipeline
