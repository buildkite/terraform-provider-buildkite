# Buildkite Terraform Provider

[![Build status](https://badge.buildkite.com/7224047dadf711cab2facd75939ea39848850d7c5c5a765acd.svg?branch=main)](https://buildkite.com/buildkite/terraform-provider-buildkite-main)

This is the official Terraform provider for [Buildkite](https://buildkite.com). The provider is listed in the [Terraform Registry](https://registry.terraform.io/) and supports Terraform >= 1.0.

The provider allows you to manage resources in your Buildkite organization.

Two configuration values are required:

-   An API token, generated at https://buildkite.com/user/api-access-tokens. The token must have the `write_pipelines`, `read_pipelines` and `write_suites` REST API scopes and be enabled for GraphQL API access.
-   A Buildkite organization slug, available by signing into buildkite.com and examining the URL: https://buildkite.com/<org-slug>.

## Documentation

The reference documentation on [the terraform registry](https://registry.terraform.io/providers/buildkite/buildkite/latest/docs) is the recommended location for guidance on using this provider.


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
2. Compile the provider with `make build`
3. Add a `.terraformrc` configuration file to override the provider binary. See below: [Overriding the provider for local development](#overriding-the-provider-for-local-development)
4. Run `terraform plan` in the tf-proj directory

    ```bash
    BUILDKITE_API_TOKEN=<api-token> BUILDKITE_ORGANIZATION_SLUG=<org-slug> terraform plan
    ```

### Overriding the provider for local development

You'll need to add a provider override to your `~/.terraformrc`. Documentation around using this file can be found [here](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers). See below for an example of a `.terraformrc` file set up for plugin override.

```hcl
provider_installation {

        dev_overrides {
                "buildkite/buildkite" = "/Path/to/this/repo/directory/"
        }

        direct {}
}

```

When you run `terraform` commands, you will now be using the local plugin, rather than the remote.

## Acceptance tests

Acceptance tests that test the provider works against the live Buildkite API can be executed like this:

```bash
make testacc
```

These tests require two environment variables to run correctly:

```bash
BUILDKITE_ORGANIZATION_SLUG=<org-slug> BUILDKITE_API_TOKEN=<token> make testacc
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
