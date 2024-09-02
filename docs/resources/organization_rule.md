---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "buildkite_organization_rule Resource - terraform-provider-buildkite"
subcategory: ""
description: |-
  An Organization Rule allows specifying explicit rules between two Buildkite resources and the desired effect/action.
  More information on pipelines can be found in the documentation https://buildkite.com/docs/pipelines/rules/overview.
---

# buildkite_organization_rule (Resource)

An Organization Rule allows specifying explicit rules between two Buildkite resources and the desired effect/action. 

More information on pipelines can be found in the [documentation](https://buildkite.com/docs/pipelines/rules/overview).

## Example Usage

```terraform
resource "buildkite_organization_rule" "trigger_build_test_dev" {
    type = "pipeline.trigger_build.pipeline"
    value = jsonencode({
        source_pipeline_uuid = buildkite_pipeline.app_dev_deploy.uuid
        target_pipeline_uuid = buildkite_pipeline.app_test_ci.uuid
    })
}

resource "buildkite_organization_rule" "artifacts_read_test_dev" {
    type = "pipeline.artifacts_read.pipeline"
    value = jsonencode({
        source_pipeline_uuid = buildkite_pipeline.app_test_ci.uuid
        target_pipeline_uuid = buildkite_pipeline.app_dev_deploy.uuid
    })
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `type` (String) The type of organization rule.
- `value` (String) The JSON document that this organization rule implements.

### Read-Only

- `action` (String) The action defined between source and target resources.
- `effect` (String) Whether this organization rule allows or denys the action to take place between source and target resources.
- `id` (String) The GraphQL ID of the organization rule.
- `source_type` (String) The source resource type that this organization rule allows or denies to invoke its defined action.
- `source_uuid` (String) The UUID of the resource that this organization rule allows or denies invocating its defined action.
- `target_type` (String) The target resource type that this organization rule allows or denies the source to respective action.
- `target_uuid` (String) The UUID of the target resourcee that this organization rule allows or denies invocation its respective action.
- `uuid` (String) The UUID of the organization rule.

## Import

Import is supported using the following syntax:

```shell
# import an organization rule resource using the rules GraphQL ID
#
# You can use this query to find the first 50 organiation rules (adjust for less or more):
# query getOrganizationRules {
#   organization(slug: "ORGANIZATION_SLUG") {
#     rules(first: 50) {
#       edges{
#         node{
#           id
#           sourceType
#           targetType
#           action
#         }
#       }
#     }
#   }
# }
#
# Depending on the speciific source/target, you're also able to filter on the source/target information
# query getOrganizationRules {
#   organization(slug: "ORGANIZATION_SLUG") {
#     rules(first: 50) {
#       edges{
#         node{
#           id
#           sourceType
#           source {
#             ... on Pipeline{
#               uuid
#               name
#             }            
#           }
#           targetType
#           target {
#             ... on Pipeline{
#               uuid
#               name
#             }            
#           }
#           action
#         }
#       }
#     }
#   }
# }

terraform import buildkite_organization_rule.artifact_read UnVsZS0tLTAxOTE5NmU2LWNiNjctNzZiZi1iYzAyLTVhYzFiNzhhMWMyOA==
```