# Resource: pipeline_team

This resource allows you to create and manage team configuration in a pipeline.

Buildkite Documentation: https://buildkite.com/docs/pipelines/permissions#permissions-with-teams-pipeline-level-permissions

## Example Usage

```hcl
resource "buildkite_pipeline_team" "developers" { 
  pipeline_id = buildkite_pipeline.repo2
  team_id = buildkite_team.test.id
  access_level = "MANAGE_BUILD_AND_READ"  
}
```

## Argument Reference

* `team_id` - (Required) The GraphQL ID of the team to add to/remove from.
* `pipeline_id` - (Required) Terraform resource ID of a buildkite pipeline (Buildkite GraphQL ID).
* `access_level` - (Required) The level of access to grant. Must be one of `READ_ONLY`, `BUILD_AND_READ` or `MANAGE_BUILD_AND_READ`.

## Attribute Reference

* `id` - The GraphQL ID of the pipeline schedule
* `uuid` - The UUID of the pipeline schedule

## Import

Pipeline teams can be imported using their `GraphQL ID`, e.g.

```
$ terraform import buildkite_pipeline_team.guests VGVhbS0tLWU1YjQyMDQyLTUzN2QtNDZjNi04MjY0LTliZjFkMzkyYjZkNQ==
```

Your pipeline team's GraphQL ID can be found with the below GraphQL query below.  
```graphql
query getPipelineTeamId {
  pipeline(slug: "ORGANIZATION_SLUG/PIPELINE_SLUG") {
		teams(first: 5, search: "PIPELINE_SEARCH_TERM") {
      edges{
        node{
          id 
        }
      }
    }
  }
}
```
