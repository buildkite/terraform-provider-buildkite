# Contributing to Buildkite Terraform Provider

We welcome contributions from the community to make the Buildkite Terraform Provider even better.

## Getting Started

To get started with contributing, please follow these steps:

1. Fork the repository
2. Create a feature branch with a descriptive name (`git checkout -b my-new-feature`)
3. Write your code
4. Commit your changes and push them to your forked repository
5. Submit a pull request with a detailed description of your changes

The Buildkite team will review your PR and start a CI build for it. The team aims to interact with every PR within a week. For security reasons, CI isn't automatically run on forked repos, and a human will review the PR before CI runs.

## Development Prerequisites

You'll need the following installed on your machine:

- **Go 1.24+**: The provider is written in Go
- **Terraform**: For testing your changes
- **Lefthook** (optional): For managing Git hooks (see [lefthook.dev](https://lefthook.dev))

Dependencies are managed via Go modules and installed automatically as required.

## Building the Provider

To compile the provider:

```bash
make build
```

This will create a `terraform-provider-buildkite` binary in the current directory.

To build release binaries for all platforms:

```bash
make build-snapshot
```

## Running Tests

### Unit Tests

Run local tests that don't require any network access:

```bash
make test
```

### Acceptance Tests

Acceptance tests test the provider against the live Buildkite API:

```bash
BUILDKITE_ORGANIZATION_SLUG=<org-slug> BUILDKITE_API_TOKEN=<token> make testacc
```

**Important notes about acceptance tests:**

- These tests make live changes to an organization and probably shouldn't be run against organizations with real data
- Anyone actively developing features for this provider is welcome to request a test organization by contacting support@buildkite.com
- The CI process will not run acceptance tests on pull requests
- Code reviewers will run the acceptance tests manually
- Please run the acceptance tests locally to confirm they pass before requesting a review

### Code Quality

Before committing, ensure your code passes these checks:

**Linting:**

```bash
golangci-lint run
```

**Formatting:**

```bash
gofmt -s -w .
```

**Vet:**

```bash
make vet
```

**Documentation:**

```bash
make docs
```

If you've installed Lefthook and your hooks (`lefthook install`), these checks will run automatically on commit.

## Manual Testing

The repo contains a `tf-proj/` directory that can be used to quickly test a compiled version of the provider from the current branch.

1. Update `tf-proj/main.tf` to use the resource or property you're developing
2. Compile the provider with `make build`
3. Add a `.terraformrc` configuration file to override the provider binary (see below)
4. Run `terraform plan` in the tf-proj directory:

```bash
BUILDKITE_API_TOKEN=<api-token> BUILDKITE_ORGANIZATION_SLUG=<org-slug> terraform plan
```

### Overriding the Provider for Local Development

Add a provider override to your `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "buildkite/buildkite" = "/path/to/terraform-provider-buildkite/"
  }

  direct {}
}
```

See the [Terraform documentation](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers) for more details.

## Development Guidelines

### API Usage

Buildkite has two APIs: REST and GraphQL. **New resources should use the GraphQL API where possible**, but can fall back to the REST API for resources or properties not yet supported by GraphQL.

### Generating GraphQL Code

If you're working with GraphQL queries:

1. Generate the schema:
   ```bash
   make schema
   ```

2. Generate the GraphQL code:
   ```bash
   make generate
   ```

### Commit Messages

- Aim for "atomic commits" - each commit should be a single logical change
- Use clear, descriptive commit messages
- Reference relevant issues in your commits

### Code Style

- Follow standard Go conventions
- Run `go fmt` on your code
- Ensure all tests pass before submitting a PR

## Continuous Integration

There is a continuous integration pipeline on Buildkite:
https://buildkite.com/buildkite/terraform-provider-buildkite-main

The CI pipeline runs:
- Linting (`golangci-lint`)
- Code vetting (`go vet`)
- Unit tests
- Acceptance tests
- Documentation generation
- Build verification
- Security scanning

## Release Process

Pushing a new version tag to GitHub (or creating a new release on github.com) will trigger a new build in the release pipeline. That pipeline will:

1. Compile the appropriate binaries
2. Sign them
3. Attach them to a draft release in https://github.com/buildkite/terraform-provider-buildkite

A maintainer will edit the draft to add the relevant changes and click publish.

The [Terraform Registry](https://registry.terraform.io) will detect the new release on GitHub and update their own listings.

## Reporting Issues

If you encounter any issues or have suggestions for improvements, please open an issue on the GitHub repository with detailed information about:

- What you were trying to do
- What happened
- What you expected to happen
- Steps to reproduce the issue
- Your environment (OS, Go version, Terraform version)

## Getting API Tokens

To develop and test the provider, you'll need an API token with permissions suitable for the actions you want to perform. It's recommended that you create a dedicated organization for Terraform plugin development, given the broad range of permissions required.

You can generate one here: https://buildkite.com/user/api-access-tokens/new?description=terraform

## Contact

If the review is taking too long, feel free to ping the team through:
- GitHub comments
- Email: support@buildkite.com

Happy contributing!
