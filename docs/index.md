# Buildkite Provider

This provider can be used to manage resources on `buildkite.com` via Terraform.

Create an API Access Token in Buildkite at https://buildkite.com/user/api-access-tokens/new.

## Example Usage

```hcl
provider "buildkite" {
    api_token = "token" # can also be set from env: BUILDKITE_API_TOKEN
    organization = "slug" # can also be set from env: BUILDKITE_ORGANIZATION
}
```

## Argument Reference

* `api_token` - (Required) This is the Buildkite API Access Token. It must be provided but can also be sourced from the `BUILDKITE_API_TOKEN` environment variable.
* `organization` - (Required) This is the Buildkite organization slug. It must be provided, but can also be sourced from the `BUILDKITE_ORGANIZATION` environment variable. The token requires GraphQL access and the `write_pipelines` scope.
