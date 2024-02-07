locals {
  repository = "git@github.com:my-org/my-repo.git"
}

data "buildkite_pipeline_template" "dev_template" {
  id = buildkite_pipeline_template.template_dev.id
}

data "buildkite_pipeline_template" "frontend_template" {
  name = "Frontend app template"
}


resource "buildkite_pipeline" "apiv2_dev" {
  name       = "API v2"
  repository = local.repository
  pipeline_template_id = data.buildkite_pipeline_template.dev_template.id
}


resource "buildkite_pipeline" "frontend" {
  name       = "Frontend"
  repository = local.repository
  pipeline_template_id = data.buildkite_pipeline_template.frontend_template.id
}