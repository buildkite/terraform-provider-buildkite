# Resource: pipeline_schedule

This resource allows you to create and manage pipeline schedules.

Buildkite Documentation: https://buildkite.com/docs/pipelines/scheduled-builds

## Example Usage

```hcl
resource "buildkite_pipeline_schedule" "repo2_nightly" {
  pipeline_id = buildkite_pipeline.repo2.id
  label       = "Nightly build"
  cronline    = "@midnight"
  branch      = buildkite_pipeline.repo2.default_branch
  timeouts {
    create = "60m"
  }
}
```

## Argument Reference

* `pipeline_id` - (Required) Terraform resource ID of a buildkite pipeline (Buildkite GraphQL ID).
* `label` - (Required) Schedule label.
* `cronline` - (Required) Schedule interval (see [docs](https://buildkite.com/docs/pipelines/scheduled-builds#schedule-intervals)).
* `branch` - (Required) The branch to use for the build.
* `commit` - (Optional, Default: `HEAD`) The commit ref to use for the build.
* `message` - (Optional, Default: `Scheduled build`) The message to use for the build.
* `env` - (Optional) A map of environment variables to use for the build.
* `enabled` - (Optional, Default: `true`) Whether the schedule should run.
* `timeouts` - (Optional) A `block` (see above) for timeout values. The default is 60s (1m).

## Attribute Reference

* `id` - The GraphQL ID of the pipeline schedule
* `uuid` - The UUID of the pipeline schedule

## Import

Pipeline schedules can be imported using their `GraphQL ID`, e.g.

```
$ terraform import buildkite_pipeline_schedule.test UGlwZWxpgm5Tf2hhZHVsZ35tLWRk4DdmN7c4LTA5M2ItNDM9YS0gMWE0LTAwZDUgYTAxYvRf49==
```

Your pipeline schedules' GraphQL ID can be found with the below GraphQL query below. Alternatively, you could use this [pre-saved query](https://buildkite.com/user/graphql/console/45687b7c-2565-4acb-8a74-750a3647875f), specifying the organisation slug (when known) and the pipeline search term (PIPELINE_SEARCH_TERM).

```graphql
query getPipelineScheduleId {
  organization(slug: "ORGANIZATION_SLUG") {
		pipelines(first: 5, search: "PIPELINE_SEARCH_TERM") {
      edges{
        node{
          name
          schedules{
            edges{ 
              node{
                id
              }
            }
          }
        }
      }
    }
  }
}
```
