# Changelog

All notable changes to this project will be documented in this file.

## [v0.2.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.1.0...v0.2.0)

### Added

* New `buildkite_pipeline` data source [[PR #106](https://github.com/buildkite/terraform-provider-buildkite/pull/106)] @yob

### Changed

* Add darwin/arm64 (M1 systems) to the build matrix [[PR #104](https://github.com/buildkite/terraform-provider-buildkite/pull/104)] @yob
* The following resources and data sources can now be used by API tokens that belong to non administrators, provided
  the token belongs to a user who has team maintainer permissions [[PR #112](https://github.com/buildkite/terraform-provider-buildkite/pull/112)] @chloeruka @yob
  * `buildkite_pipeline` resource
  * `buildkite_pipeline` data source
  * `buildkite_pipeline_schedule` resource
* All resources and data sources now have acceptance tests [many PRs] @chloeruka @yob

## [v0.1.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.0.17...v0.1.0)

### Added

* New `pipeline_schedule` resource [[PR #87](https://github.com/buildkite/terraform-provider-buildkite/pull/87)] @vgrigoruk

### Changed

* Require terraform 0.13 or greater [[PR #89](https://github.com/buildkite/terraform-provider-buildkite/pull/89)] @vgrigoruk
* Add PowerPC 64 LE to the build matrix [[PR #92](https://github.com/buildkite/terraform-provider-buildkite/pull/92)] @runlevel5

## [v0.0.17](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.0.16...v0.0.17)

* No code changes from 0.0.16 - just the first release signed by a buildkite gpg key that will be
  available in the terraform registry as buildkite/buildkite.
