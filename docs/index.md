# Buildkite Provider

This provider can be used to manage resources on [buildkite.com](https://buildkite.com) via Terraform.

Two configuration values are required:

-   An API token, generated at https://buildkite.com/user/api-access-tokens. The
    token must have the `write_pipelines`, `read_pipelines` and `write_suites` REST API scopes and also be enabled for GraphQL
-   A Buildkite organization slug, available by signing into buildkite.com and
    examining the URL: https://buildkite.com/<org-slug>

## Example Usage

```hcl
terraform {
  required_providers {
    buildkite = {
      source  = "buildkite/buildkite"
      version = "0.26.0"
    }
  }
}

provider "buildkite" {
  api_token    = "token" # can also be set from env: BUILDKITE_API_TOKEN
  organization = "slug"  # can also be set from env: BUILDKITE_ORGANIZATION_SLUG
  timeouts { # override the default timeout of 30s for the create and update operations for all resources
    create = "10s"
    update = "15s"
  }
}
```

## Argument Reference

- `api_token` - (Required) This is the Buildkite API Access Token. It must be provided but can also be sourced from the `BUILDKITE_API_TOKEN` environment variable.
- `organization` - (Required) This is the Buildkite organization slug. It must be provided, but can also be sourced from the `BUILDKITE_ORGANIZATION_SLUG` environment variable. The token requires GraphQL access and the `write_pipelines`, `read_pipelines` and `write_suites` REST API scopes.
- `graphql_url` - (Optional) This is the base URL to use for GraphQL requests. It defaults to "https://graphql.buildkite.com/v1", but can also be sourced from the `BUILDKITE_GRAPHQL_URL` environment variable.
- `rest_url` - (Optional) This is the the base URL to use for REST requests. It defaults to "https://api.buildkite.com", but can also be sourced from the `BUILDKITE_REST_URL` environment variable.
- `archive_pipeline_on_delete` - (Optional) Whether to archive pipelines when being destroyed instead of deleting them. This can be used a soft-delete approach to pipeline destruction.
- `timeouts` - (Optional. Default `30s`) A block of `create`, `read`, `update`, and `delete` durations. These are used by the provider per resource when running through the given CRUD operation.

## Usage of `timeouts` and retries

All resources and datasources managed by this provider have a global timeout configuration. This timeout is used to limit when performing the CRUD operations on each resource.
It is configured globally for the provider but each resource uses the timeout separately. Ie. if you manage 3 pipelines in terraform and have set an update timeout of 15 seconds, each pipeline will be given 15 seconds to finish updating (not 15/3 (5) seconds to update).

CRUD operations also have retries configured with an exponential back-off. The retry attempts are included in the timeout. Ie. with a timeout of 15 seconds, a pipeline create operation will retry as many times as it can within 15 seconds.
This helps to automatically retry operations when the API is experiencing high latency.
