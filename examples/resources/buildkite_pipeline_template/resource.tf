resource "buildkite_pipeline_template" "template_required" {
    name = "QA upload"
    configuration = "steps:\n  - label: \":pipeline:\"\n    command: \"buildkite-agent pipeline upload .buildkite/pipeline-qa.yml\"\n"
}

resource "buildkite_pipeline_template" "template_full" {
    name = "Production upload"
    description = "Production upload template"
    configuration = "steps:\n  - label: \":pipeline:\"\n    command: \"buildkite-agent pipeline upload .buildkite/pipeline-production.yml\"\n"
    available = true
}