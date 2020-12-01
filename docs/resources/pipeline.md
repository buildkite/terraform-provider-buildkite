# Resource: pipeline

This resource allows you to create and manage pipelines for repositories.

Buildkite Documentation: https://buildkite.com/docs/pipelines

## Example Usage

```hcl
# in ./steps.yml:
# steps:
#   - label: ':pipeline:'
#     command: buildkite-agent upload

resource "buildkite_pipeline" "repo2" {
    name = "repo2"
    repository = "git@github.com:org/repo2"
    steps = file("./steps.yml")

    team {
        slug = "everyone"
        access_level = "READ_ONLY"
    }
}
```

## Argument Reference

* `name` - (Required) The name of the pipeline.
* `repository` - (Required) The git URL of the repository.
* `steps` - (Required) The string YAML steps to run the pipeline.
* `description` - (Optional) A description of the pipeline.

* `default_branch` - (Optional) The default branch to prefill when new builds are created or triggered.
* `branch_configuration` - (Optional) Limit which branches and tags cause new builds to be created, either via a code push or via the Builds REST API.
* `skip_intermediate_builds` - (Optional, Default: `false` ) A boolean to enable automatically skipping any unstarted builds on the same branch when a new build is created.    
* `skip_intermediate_builds_branch_filter` - (Optional) Limit which branches build skipping applies to, for example !master will ensure that the master branch won't have it's builds automatically skipped.
* `cancel_intermediate_builds` - (Optional, Default: `false` ) A boolean to enable automtically cancelling any running builds on the same branch when a new build is created.   
* `cancel_intermediate_builds_branch_filter` - (Optional) Limit which branches build cancelling applies to, for example !master will ensure that the master branch won't have it's builds automatically cancelled.
* `team` - (Optional) Set team access for the pipeline. Can be specified multiple times for each team.

### Team

The `team` block supports:

* `slug` - (Required) The buildkite slug of the team.
* `access_level` - (Required) The level of access to grant. Must be one of `READ_ONLY`, `BUILD_AND_READ` or `MANAGE_BUILD_AND_READ`.

## Attribute Reference

* `webhook_url` - The Buildkite webhook URL to configure on the repository to trigger builds on this pipeline.
* `slug` - The slug of the created pipeline.


## Import

Pipelines can be imported using the `GraphQL ID` (not UUID), e.g.

```
$ terraform import buildkite_pipeline.fleet UGlwZWxpbmUtLS00MzVjYWQ1OC1lODFkLTQ1YWYtODYzNy1iMWNmODA3MDIzOGQ=
```