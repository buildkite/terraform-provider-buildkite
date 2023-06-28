# Changelog

All notable changes to this project will be documented in this file.

## Unreleased
* Support oftion to archive on delete [[PR #296](https://github.com/buildkite/terraform-provider-buildkite/pull/296)]

## [v0.19.2](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.19.1...v0.19.2)
* Consistent naming for environment variables [[PR #290](https://github.com/buildkite/terraform-provider-buildkite/pull/290)] @mcncl
* Support TF version < 0.15.4 [[PR #294](https://github.com/buildkite/terraform-provider-buildkite/pull/294)] @mcncl

## [v0.19.1](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.19.0...v0.19.1)

* SUP-202 Add graphql example queries for finding import IDs [[PR #280](https://github.com/buildkite/terraform-provider-buildkite/pull/280)] @james2791 @jradtilbrook
* SUP-1072 Create new provider using framework plugin [[PR #286](https://github.com/buildkite/terraform-provider-buildkite/pull/286)] @jradtilbrook
* SUP-1066 Migrate agent token to framework [[PR #289](https://github.com/buildkite/terraform-provider-buildkite/pull/289)] @jradtilbrook
* Omit empty buildRetentionEnabled input [[PR #291](https://github.com/buildkite/terraform-provider-buildkite/pull/291)] @jradtilbrook

## [v0.19.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.18.0...v0.19.0)

### Breaking changes
This release removes the ability to import Agent tokens.  
As per https://buildkite.com/changelog/207-agent-token-being-deprecated-from-graphql-apis, it will soon not be possible
to read the agent token value after creation, making importing Agent tokens impossible.

**Note**: If you are using an earlier version than `v0.19.0` after the change above occurs, you will likely see an
unexpected diff in your `terraform plan`s. Upon state refresh, the token values will be emptied out which could trigger
other dependent resources to change. It is highly recommended to upgrade to `v0.19.0` prior to avoid this happening.

### Fixes

* Pipelines resource Computed/Default nil values reversion [[PR #277](https://github.com/buildkite/terraform-provider-buildkite/pull/277)] @james2791
* Allow pipeline to be removed from a cluster [[PR #279](https://github.com/buildkite/terraform-provider-buildkite/pull/279)] @jradtilbrook
* Change default provider settings to match new pipeline [[PR #282](https://github.com/buildkite/terraform-provider-buildkite/pull/282)] @jradtilbrook

### Added

* Agent Token resource genqlient migration & adjustment [[PR #281](https://github.com/buildkite/terraform-provider-buildkite/pull/281)] @james2791

## [v0.18.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.17.1...v0.18.0)

### Added

* DOCS fmt [[PR #271](https://github.com/buildkite/terraform-provider-buildkite/pull/271)] @mattclegg
* Add PR/issue templates [[PR #272](https://github.com/buildkite/terraform-provider-buildkite/pull/272)] @jradtilbrook
* Add bug report issue template [[PR #273](https://github.com/buildkite/terraform-provider-buildkite/pull/273)] @jradtilbrook
* Use gotestsum for running and reporting tests [[PR #274](https://github.com/buildkite/terraform-provider-buildkite/pull/274)] @jradtilbrook
* Remove Computed values from Pipeline where not required [[PR #275](https://github.com/buildkite/terraform-provider-buildkite/pull/275)] @mcncl 
* SUP-995: Changelog updates, v0.18.0 release prep [[PR #276](https://github.com/buildkite/terraform-provider-buildkite/pull/276)] @james2791

### Fixes
* Fixed issues when bumping genqlient from 0.4.0 to 0.6.0 [[PR #278](https://github.com/buildkite/terraform-provider-buildkite/pull/278)] @lizrabuya

## [v0.17.1](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.17.0...v0.17.1)

### Fixed

* SUP-906/Fix: Adjustment of README version, Pipeline Argument Reference amendments [[PR #269](https://github.com/buildkite/terraform-provider-buildkite/pull/269)] @james2791

## [v0.17.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.16.0...v0.17.0)

### Added

* SUP-707: Pipeline resource's steps made optional, default pipeline upload step [[PR #265](https://github.com/buildkite/terraform-provider-buildkite/pull/265)] @james2791
* Update the paths where we load environment variables from [[PR #266](https://github.com/buildkite/terraform-provider-buildkite/pull/265)] @yob

## [v0.16.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.15.0...v0.16.0)

### Added

* Allow release pipeline to pull from private ECR registry [[PR #258](https://github.com/buildkite/terraform-provider-buildkite/pull/258)] @ellsclytn
* Set User-Agent header on both REST and graphql requests [[PR #259](https://github.com/buildkite/terraform-provider-buildkite/pull/259)] @yob
* Update terraform SDKv2 to the latest version [[PR #261](https://github.com/buildkite/terraform-provider-buildkite/pull/261)] @yob 
* Run go mod tidy [[PR #262](https://github.com/buildkite/terraform-provider-buildkite/pull/262)] @yob
* SUP-819 Retrieve org id only once in provider configuration [[PR #263](https://github.com/buildkite/terraform-provider-buildkite/pull/263)] @jradtilbrook 
* SUP-820 Pull latest 1.4 version terraform from docker image [[PR #264](https://github.com/buildkite/terraform-provider-buildkite/pull/264)] @jradtilbrook

## [v0.15.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.14.0...v0.15.0)

### Added

* Don't include null body in GET requests [[PR #254](https://github.com/buildkite/terraform-provider-buildkite/pull/254)] @nhurden
* Set User-Agent header in client [[PR #256](https://github.com/buildkite/terraform-provider-buildkite/pull/256)] @danstn

## [v0.14.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.12.2...v0.14.0)

### Added

* Update Go and the package dependendies (inc SUP-841) [[PR #240](https://github.com/buildkite/terraform-provider-buildkite/pull/240)] @mcncl
* SUP-857 Manually update go getter & deps [[PR #246](https://github.com/buildkite/terraform-provider-buildkite/pull/246)] @mcncl
* Add documentation around plugin override for local dev [[PR #247](https://github.com/buildkite/terraform-provider-buildkite/pull/247)] @mcncl
* SUP-838 Ensure CI passes/fails correctly [[PR #248](https://github.com/buildkite/terraform-provider-buildkite/pull/248)] @jradtilbrook
* SUP-866 Add deletion protection for a pipeline [[PR #250](https://github.com/buildkite/terraform-provider-buildkite/pull/250)] @mcncl

### Fixes

* SUP-770 Fix panic in `GetOrganizationID` [[PR #249](https://github.com/buildkite/terraform-provider-buildkite/pull/249)] @jradtilbrook

## [v0.12.2](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.12.1...v0.12.2)

### Added

* Add a reviewer to dependabot PRs [[PR #236](https://github.com/buildkite/terraform-provider-buildkite/pull/236)] @yob
* Add support for timeout settings. [[PR #238](https://github.com/buildkite/terraform-provider-buildkite/pull/238)] @mcncl

## [v0.12.1](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.12.0...v0.12.1)

### Added

* Add `organization_settings` documentation [[PR #237](https://github.com/buildkite/terraform-provider-buildkite/pull/237)] @mcncl

## [v0.12.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.11.1...v0.12.0)

### Added

* Added Organisational settings [[PR #230](https://github.com/buildkite/terraform-provider-buildkite/pull/230)] @mcncl

## [v0.11.1](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.11.0...v0.11.1)

* Documentation had fallen out of sync due to lack of tag use. This change in the version includes a tag bump in order to merge documentation changes/improvements in to the provider page.

###
* GraphQL demo with Khan/genqlient [[PR #205](https://github.com/buildkite/terraform-provider-buildkite/pull/205)] @jradtilbrook 
* Add `allow_rebuilds documentation`. Grammar. [[#PR 208](https://github.com/buildkite/terraform-provider-buildkite/pull/208)] @jmctune 
* SUP-191 Generate test coverage [[PR #206](https://github.com/buildkite/terraform-provider-buildkite/pull/206)] @jradtilbrook  
* Add arch command for agnostic support of architecture in build command [[PR #228](https://github.com/buildkite/terraform-provider-buildkite/pull/228)] @mcncl 
* Update version and tag so that documentation is merged in to provider [[PR #229](https://github.com/buildkite/terraform-provider-buildkite/pull/229)] @mcncl 

## [v0.11.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.10.0...v0.11.0)

### Added

* Allow configuring API endpoints [[PR #202](https://github.com/buildkite/terraform-provider-buildkite/pull/202)] @jradtilbrook 

## [v0.10.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.9.0...v0.10.0)

### Added

* Allow tests to run on PRs [[PR #197](https://github.com/buildkite/terraform-provider-buildkite/pull/197)] @jradtilbrook

### Fixed

* Fix cluster support [[#PR 200](https://github.com/buildkite/terraform-provider-buildkite/pull/200)] @jradtilbrook

## [v0.9.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.8.0...v0.9.0)

### Added
* Add team data source [[PR #190](https://github.com/buildkite/terraform-provider-buildkite/pull/190)] @margueritepd 
* add support for tags [[PR #186](https://github.com/buildkite/terraform-provider-buildkite/pull/186)] @mhornbacher
* Add allow rebuilds to pipeline resource [[#PR 193](https://github.com/buildkite/terraform-provider-buildkite/pull/193)] @jradtilbrook

## [v0.8.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.7.0...v0.8.0)

### Added

* Add `cluster_id` argument to pipeline resource [[PR #181](https://github.com/buildkite/terraform-provider-buildkite/pull/181)] @kate-syberspace

## [v0.7.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.6.0...v0.7.0)

### Fixed

* Brought back build target for darwin/arm64 [[PR #189](https://github.com/buildkite/terraform-provider-buildkite/pull/189)] @mhornbacher

## [v0.6.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.5.0...v0.6.0)

### Added

* `buildkite_team_member` resource: Manage organisation team membership [[PR #173](https://github.com/buildkite/terraform-provider-buildkite/pull/173)] @jradtilbrook
Bump Golang to 1.17.3
* Pipeline resource: Add build_pull_request_labels_changed property [[PR #164](https://github.com/buildkite/terraform-provider-buildkite/pull/164)] @hadusam

### Fixed

* Fixed typo in pipeline docs [[PR #172](https://github.com/buildkite/terraform-provider-buildkite/pull/172)] @RussellRollins
* Added `cancel_deleted_branch_builds` to pipeline docs [[PR #160](https://github.com/buildkite/terraform-provider-buildkite/pull/160)] @keith

## [v0.5.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.4.0...v0.5.0)

### Added

* pipeline resource: Add badge_url property [[PR #151](https://github.com/buildkite/terraform-provider-buildkite/pull/151)] @JPScutt
* pipeline resource: Add filter_condition and filter_enabled to provider_settings [[PR #157](https://github.com/buildkite/terraform-provider-buildkite/pull/157)] @gu-kevin

### Fixed

* Improved documentation for pipeline resource [[PR #145](https://github.com/buildkite/terraform-provider-buildkite/pull/145)] [[PR #146](https://github.com/buildkite/terraform-provider-buildkite/pull/146)] @jlisee
* Improved error when an unrecognised team slug is used in pipeline resource [[PR #155](https://github.com/buildkite/terraform-provider-buildkite/pull/155)] @yob
* Improved error message when an unrecognised ID is used while importing a pipeline schedule [[PR #144](https://github.com/buildkite/terraform-provider-buildkite/pull/144)] @yob

## [v0.4.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.3.0...v0.4.0)

### Added

* `buildkite_meta` data source for fetching the IP addresses Buildkite uses for webhooks [[PR #136](https://github.com/buildkite/terraform-provider-buildkite/pull/136)] @yob

## [v0.3.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.2.0...v0.3.0)

### Added

* `buildkite_pipeline` resource can now manage provider settings (which webhooks to build on, etc) [[PR #123](https://github.com/buildkite/terraform-provider-buildkite/pull/123)] @vgrigoruk

### Changed

* `buildkite_pipeline` resource: use 'Computed: true' for attributes that are initialized on backend [[PR #125](https://github.com/buildkite/terraform-provider-buildkite/pull/125)] @vgrigoruk
  * when these properties are unspecified in terraform, default values are left to Buildkite to define

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
