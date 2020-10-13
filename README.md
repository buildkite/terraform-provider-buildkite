# Buildkite Terraform Provider

[![Build Status](https://travis-ci.com/jradtilbrook/terraform-provider-buildkite.svg?branch=master)](https://travis-ci.com/jradtilbrook/terraform-provider-buildkite)

This is a Terraform provider for Buildkite.

The provider allows you to manage resources in your Buildkite organization.

## Quick Start

- [Using the provider](https://registry.terraform.io/providers/jradtilbrook/buildkite/latest)

## Installation

The recommended way is to download a pre-built release from Github.

For OSX:

```bash
export TF_BK_VERSION=0.0.10
mkdir -p ~/.terraform.d/plugins/darwin_amd64
wget \
  "https://github.com/jradtilbrook/terraform-provider-buildkite/releases/download/v"$TF_BK_VERSION"/terraform-provider-buildkite_v"$TF_BK_VERSION"_darwin_amd64.tar.gz" \
  -O terraform-provider-buildkite.tar.gz
tar -zxvf terraform-provider-buildkite.tar.gz terraform-provider-buildkite_v$TF_BK_VERSION
mv terraform-provider-buildkite_v$TF_BK_VERSION ~/.terraform.d/plugins/darwin_amd64/
rm terraform-provider-buildkite.tar.gz
```

Other ways to get a binary for this provider will not include the version information and so are not recommended.

- clone this repo and build it
- `go get github.com/jradtilbrook/terraform-provider-buildkite`

Once you have a binary you need to make sure it's on terraform plugin search path. You can get more information from
[the terraform docs](https://www.terraform.io/docs/configuration/providers.html#third-party-plugins).

#### Releasing a version

This repo has GitHub Actions setup that will automatically build the binary for different platforms and attach it to a
GitHub release. All that is required is to create the release in GitHub.

## License

Buildkite Terraform Provider is licensed under the MIT license.

## Local development

Contributions welcome!!