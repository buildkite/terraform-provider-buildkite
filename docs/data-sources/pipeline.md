# Data Source: pipeline

Use this data source to look up properties on a specific pipeline. This is
particularly useful for looking up the webhook URL for each pipeline.

Buildkite Documentation: https://buildkite.com/docs/pipelines

## Example Usage

```hcl
data "buildkite_pipeline" "repo2" {
    slug = "repo2"
}
```

## Argument Reference

The following arguments are supported:

* `slug` - (Required) The slug of the pipeline, available in the URL of the pipeline on buildkite.com

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `description` - The description of the pipeline.
* `default_branch` - The default branch to prefill when new builds are created or triggered, usually main or master but can be anything.
* `id` - The GraphQL ID of the pipeline.
* `name` - The name of the pipeline.
* `repository` - The git URL of the repository.
* `webhook_url` - The Buildkite webhook URL to configure on the repository to trigger builds on this pipeline.
