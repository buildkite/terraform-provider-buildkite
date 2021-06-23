# Buildkite Terraform Provider

[![Build status](https://badge.buildkite.com/7224047dadf711cab2facd75939ea39848850d7c5c5a765acd.svg?branch=main)](https://buildkite.com/buildkite/terraform-provider-buildkite-main)

This is the official Terraform provider for [Buildkite](https://buildkite.com). The provider is listed in the [Terraform Registry](https://registry.terraform.io/) and supports terraform >= 0.13.

The provider allows you to manage resources in your Buildkite organization.

Two configuration values are required:

-   An API token, generated at https://buildkite.com/user/api-access-tokens. The
    token must have the `write_pipelines, read_pipelines` REST API scopes and be enabled for GraphQL
-   A Buildkite organization slug, available by signing into buildkite.com and
    examining the URL: https://buildkite.com/<org-slug>

## Documentation

The reference documentation on [the terraform registry](https://registry.terraform.io/providers/buildkite/buildkite/latest/docs)
is the recommended location for guidance on using this provider.

## Installation

**NOTE**: This provider is built with the assumption that teams are enabled for your Buildkite organization. Most resources should work without, but we can't guarantee compatibility. Check out our [documentation regarding teams](https://buildkite.com/docs/pipelines/permissions#permissions-with-teams) for more information.

To use the provider, add the following terraform:

```hcl
terraform {
  required_providers {
    buildkite = {
      source = "buildkite/buildkite"
      version = "0.5.0"
    }
  }
}

provider "buildkite" {
  # Configuration options
  api_token = "token" # can also be set from env: BUILDKITE_API_TOKEN
  organization = "slug" # can also be set from env: BUILDKITE_ORGANIZATION
}
```

## Thanks :heart:

A massive thanks to [Jarryd Tilbrook](https://github.com/jradtilbrook) for authoring the original Buildkite Terraform provider.

## License

Buildkite Terraform Provider is licensed under the MIT license.

## Local development

Contributions are welcome.

If you wish to work on the provider, you'll first need Go installed on your machine (version 1.14+ is required). Dependencies are managed via gomodules and installed automatically as required.

To compile the provider:

    make build

To run local tests that don't require any network access:

    make test

Buildkite has two APIs: REST and GraphQL. New resources should use the GraphQL API where possible, but can fallback to the REST API for resouces or properties not yet supported by GraphQL.

## Manual testing

The repo contains a tf-proj/ directory that can be used to quickly test a compiled version of the provider from the current branch.

1. Update tf-proj/main.tf to use the resource or property you're developing
2. Compile the provider and copy it into the filesystem cache in tf-proj

    ```bash
    go build -o terraform-provider-buildkite_v0.0.18 . && \
      mkdir -p tf-proj/terraform.d/plugins/registry.terraform.io/buildkite/buildkite/0.0.18/linux_amd64/ && \
      mv terraform-provider-buildkite_v0.0.18 tf-proj/terraform.d/plugins/registry.terraform.io/buildkite/buildkite/0.0.18/linux_amd64/
    ```

3. Ensure the version number in the above command and in tf-proj/main.tf match
4. Run `terraform plan` in the tf-proj directory

    ```bash
    BUILDKITE_API_TOKEN=<api-token> BUILDKITE_ORGANIZATION=<org-slug> terraform plan
    ```

## Acceptance tests

Acceptance tests that test the provider works against the live Buildkite API can be executed like this:

```bash
make testacc
```

These tests require two environment variables to run correctly:

```bash
BUILDKITE_ORGANIZATION=<org-slug> BUILDKITE_API_TOKEN=<token> make testacc
```

Note that these tests make live changes to an organization and probably
shouldn't be run against organizations with real data. Anyone actively
developing features for this provider is welcome to request a test organization
by contacting support@buildkite.com.

Also note that the CI process will not run acceptance tests on pull requests.
Code reviewers will run the acceptance tests manually, and we ask that code
submissions run the acceptance tests locally to confirm the tests pass before
requesting a review.

## Release Process

Pushing a new version tag to GitHub (or creating a new release on github.com)
will trigger a new build in the release pipeline. That pipeline will compile
the appropriate binaries, sign them, and attach them to a draft release in
https://github.com/buildkite/terraform-provider-buildkite.

Edit the draft to add in the relevant changes and click publish.

The [terraform registry](https://registry.terraform.io) will detect the new
release on GitHub and update their own listings.
