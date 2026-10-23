data "buildkite_pipeline" "receiver" {
  slug = "webhook-receiver"
}

resource "buildkite_pipeline_webhook_trigger" "deploy" {
  pipeline_slug = data.buildkite_pipeline.receiver.slug
  label         = "Deploy from an external webhook"
  enabled       = true

  build = {
    branch  = "main"
    commit  = "HEAD"
    message = "Deployment requested by webhook"
    environment = {
      TARGET = "staging"
    }
  }
}

output "webhook_endpoint" {
  value     = buildkite_pipeline_webhook_trigger.deploy.endpoint_url
  sensitive = true
}
