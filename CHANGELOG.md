# Changelog

All notable changes to this project will be documented in this file.

## [v0.27.2](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.27.1...v0.27.2)

- Ignore default team in teams block [[PR #484](https://github.com/buildkite/terraform-provider-buildkite/pull/484)] @jradtilbrook

## [v0.27.1](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.27.0...v0.27.1)

- SUP-1853 backport default_team_id to 0.27 [[PR #481](https://github.com/buildkite/terraform-provider-buildkite/pull/481)] @jradtilbrook

## [v0.27.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.26.0...v0.27.0)

- SUP-805: SUP-805: Team resource retries [[PR #360](https://github.com/buildkite/terraform-provider-buildkite/pull/360)] @james2791
- SUP-1392: Random test suite names in tests/t.Run() conversion [[PR #376](https://github.com/buildkite/terraform-provider-buildkite/pull/376)] @james2791
- SUP-1374 Add timeout to provider and cluster datasource [[PR #363](https://github.com/buildkite/terraform-provider-buildkite/pull/363)] @jradtilbrook
- SUP-804: Retry pipeline_schedule api requests[[PR #380](https://github.com/buildkite/terraform-provider-buildkite/pull/380)] @lizrabuya
- SUP-1374 Remove timeout context [[PR #378](https://github.com/buildkite/terraform-provider-buildkite/pull/378)] @jradtilbrook
- SUP-1400: Test suite and test suite team (team suite) resource retries [[PR #383](https://github.com/buildkite/terraform-provider-buildkite/pull/383)] @james2791
- SUP-1322: Team member resource retries [[PR #381](https://github.com/buildkite/terraform-provider-buildkite/pull/381)] @james2791
- SUP-1402: Agent token resource retries [[PR #382](https://github.com/buildkite/terraform-provider-buildkite/pull/382)] @james2791
- SUP-1399: Add retry to pipeline team resource [[PR #384](https://github.com/buildkite/terraform-provider-buildkite/pull/384)] @lizrabuya
- SUP-1401: Cluster Queue and Cluster Agent Token resource retries [[PR #388](https://github.com/buildkite/terraform-provider-buildkite/pull/388)] @james2791
- SUP-1361: Add timeouts to pipeline resource api [[PR #385](https://github.com/buildkite/terraform-provider-buildkite/pull/385)] @lizrabuya
- SUP-1393 Detect repository provider type [[PR #386](https://github.com/buildkite/terraform-provider-buildkite/pull/386)] @jradtilbrook
- SUP-1405: Fix dangling team resources created in tests [[PR #389](https://github.com/buildkite/terraform-provider-buildkite/pull/389)] @lizrabuya

### Changes

This release introduces a default timeout for all CRUD operations on resources of 30 seconds. You can override this using the `timeout` attribute of the provider configuration block.

## [v0.26.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.25.0...v0.26.0)

- SUP-1375 Use context everywhere [[PR #362](https://github.com/buildkite/terraform-provider-buildkite/pull/362)] @jradtilbrook
- SUP-1319: Removal of archive_on_delete from pipeline resource [[PR #369](https://github.com/buildkite/terraform-provider-buildkite/pull/369)] @james2791
- SUP-1383 Fix teams block bug in v0.25.0 [[PR #370](https://github.com/buildkite/terraform-provider-buildkite/pull/370)] @jradtilbrook
- SUP-1320 Remove deletion_protection from pipeline resource [[PR #373](https://github.com/buildkite/terraform-provider-buildkite/pull/373)] @lizrabuya
- SUP-1337 Remove org settings resource [[PR #368](https://github.com/buildkite/terraform-provider-buildkite/pull/368)] @lizrabuya
- SUP-1380 Use ID for Cluster importing [[PR #372](https://github.com/buildkite/terraform-provider-buildkite/pull/372)] @mcncl
- SUP-1388 Implement planmodifier.String for slugs [[PR #374](https://github.com/buildkite/terraform-provider-buildkite/pull/374)] @jradtilbrook

### Changes

The `archive_on_delete` attribute has been removed from the `buildkite_pipeline` resource in this release. Please use the provider configuration `archive_pipeline_on_delete` instead.

The `deletion_protection` attribute has also been removed from the `buildkite_pipeline` resource in this release. This feature offers similar
functionality to [lifecycles](https://developer.hashicorp.com/terraform/language/meta-arguments/lifecycle) which are supported by Terraform.

## [v0.25.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.24.0...v0.25.0)

- Move archive pipeline config to provider [[PR #354](https://github.com/buildkite/terraform-provider-buildkite/pull/354)] @jradtilbrook
- SUP-1076 Convert testing to framework [[PR #361](https://github.com/buildkite/terraform-provider-buildkite/pull/361)] @jradtilbrook
    - SUP-1076 Move Cluster tests to use t.Run [[PR #365](https://github.com/buildkite/terraform-provider-buildkite/pull/365)] @mcncl
- SUP-1368 Fix pipeline resource update [[PR #359](https://github.com/buildkite/terraform-provider-buildkite/pull/359)] @lizrabuya
- SUP-1307: Implement Pipeline Team Resource[[PR #351](https://github.com/buildkite/terraform-provider-buildkite/pull/351)] @lizrabuya

This release implements the `buildkite_pipeline_team` resource to create and manage team configuration in a pipeline. Tests have also been refactored to use Framework from SDKv2.

## [v0.24.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.23.0...v0.24.0)

- SUP-1068 Migrate pipeline resource to framework [[PR #345](https://github.com/buildkite/terraform-provider-buildkite/pull/345)] @jradtilbrook
- Bump github.com/hashicorp/terraform-plugin-framework from 1.3.3 to 1.3.4 [[PR #349](https://github.com/buildkite/terraform-provider-buildkite/pull/349)]
- Bump github.com/hashicorp/terraform-plugin-framework-validators [[PR #350](https://github.com/buildkite/terraform-provider-buildkite/pull/350)]
- refactor 🧹: Refactor templates to use Conventional Commits[[PR #348](https://github.com/buildkite/terraform-provider-buildkite/pull/348)] @mcncl

This release migrates `buildkite_pipeline` to the terraform plugin framework. Every effort was made to maintain
backwards compatibility with the provider. Due to these changes, there are some transparent changes to the state file.
This should not cause any errors for end-users, however if you find a problem, please raise an issue.

## [v0.23.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.22.0...v0.23.0)

### Added

- SUP-1305: Test Suite Team Resource addition [[PR #346](https://github.com/buildkite/terraform-provider-buildkite/pull/346)] @james2791
- SUP-1301: Deletion protection deprecation warning [[PR #342](https://github.com/buildkite/terraform-provider-buildkite/pull/342)] @mcncl

### Fixed

- Fixed a bug in `buildkite_test_suite` resources where `team_owner_id` could be set to the `access_level` instead @james2791 @jradtilbrook

### Forthcoming Changes
`deletion_protection` is being deprecated and will be removed in a future release (`v1`). This feature offers similar
functionality to [lifecycles](https://developer.hashicorp.com/terraform/language/meta-arguments/lifecycle) which are supported by Terraform.

## [v0.22.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.21.2...v0.22.0)

### Added 

- SUP-1281: Pipeline resource ReadPipeline conversion to Genqlient[[PR #319](https://github.com/buildkite/terraform-provider-buildkite/pull/319)] @james2791
- Convert CreatePipeline to genqlient [[PR #334](https://github.com/buildkite/terraform-provider-buildkite/pull/334)] @jradtilbrook
- SUP-196: Pipeline Schedule GraphQL transition [[PR #339](https://github.com/buildkite/terraform-provider-buildkite/pull/339)] @james2791
- Add buildkite_test_suite resource [[PR #327](https://github.com/buildkite/terraform-provider-buildkite/pull/327)] @jradtilbrook

### Forthcoming Changes
`deletion_protection` is being deprecated and will be removed in a future release (`v1`). This feature offers similar
functionality to [lifecycles](https://developer.hashicorp.com/terraform/language/meta-arguments/lifecycle) which are supported by Terraform.

## [v0.21.2](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.21.1...v0.21.2)

### Fixed

- Make descriptions not required on Team [[PR #331](https://github.com/buildkite/terraform-provider-buildkite/pull/331)] @mcncl
- Set pipeline_schedule enabled=true [[PR #332](https://github.com/buildkite/terraform-provider-buildkite/pull/332)] @jradtilbrook

## [v0.21.1](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.21.0...v0.21.1)

### Fixed

-   Fix: goreleaser and Terraform manifest [[PR #330](https://github.com/buildkite/terraform-provider-buildkite/pull/330)] @james2791

## [v0.21.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.20.0...v0.21.0)

### Added

-   SUP-1052: Migrate Pipeline Schedule resource to Genqlient/Framework [[PR #320](https://github.com/buildkite/terraform-provider-buildkite/pull/320)] @lizrabuya
-   Migrate organization datasource to plugin framework [[PR #304](https://github.com/buildkite/terraform-provider-buildkite/pull/304)] @jradtilbrook
-   SUP-1051: Team Member resource Framework/Genqlient conversion [[PR #313](https://github.com/buildkite/terraform-provider-buildkite/pull/313)] @james2791
-   SUP-1067: Organization resource conversion to Framework[[PR #311](https://github.com/buildkite/terraform-provider-buildkite/pull/311)] @james2791
-   SUP-1063 Convert meta datasource to framework [[PR #314](https://github.com/buildkite/terraform-provider-buildkite/pull/314)] @jradtilbrook
-   SUP-1065: Pipeline Datasource conversion to Framework [[PR #315](https://github.com/buildkite/terraform-provider-buildkite/pull/315)] @james2791
-   Convert DeletePipeline to genqlient [[PR #317](https://github.com/buildkite/terraform-provider-buildkite/pull/317)] @jradtilbrook
-   SUP-1049 Migrate Team to framework & Genqlient [[PR #318](https://github.com/buildkite/terraform-provider-buildkite/pull/317)] @mcncl

### Forthcoming changes

This release deprecates the `buildkite_organization_settings` [resource](./docs/resources/organization_settings.md). In a future minor release, we will remove this resource in favour of the newer `buildkite_organization` [resource](./docs/resources/organization.md) that aligns with the [datasource](./docs/data-sources/organization.md) of the same name.

PR [#318](https://github.com/buildkite/terraform-provider-buildkite/pull/318) introduces a change to make it easier to use data-sources with Teams; both `slug` and `id` are now accepted as arguments. Only one of either `slug` or `id` should be set in order to use the `data.buildkite_team` data-source.


## [v0.20.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.19.2...v0.20.0)

### Added

-   Support option to archive on delete [[PR #296](https://github.com/buildkite/terraform-provider-buildkite/pull/296)] @mcncl
-   SUP-1085: Cluster Queue resource implementation [[PR #297](https://github.com/buildkite/terraform-provider-buildkite/pull/297)] @james2791
-   SUP-1084 Add Cluster resource [[PR #301](https://github.com/buildkite/terraform-provider-buildkite/pull/301)] @mcncl
-   Add cluster datasource [[PR #303](https://github.com/buildkite/terraform-provider-buildkite/pull/303)] @jradtilbrook
-   SUP-1086 Add cluster agent token resource [[PR #309](https://github.com/buildkite/terraform-provider-buildkite/pull/309)] @lizrabuya

### Fixed

-   SUP-270 Fix branch_configuration updating to empty string [[PR #298](https://github.com/buildkite/terraform-provider-buildkite/pull/298)] @jradtilbrook

## [v0.19.2](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.19.1...v0.19.2)

### Added

-   Consistent naming for environment variables [[PR #290](https://github.com/buildkite/terraform-provider-buildkite/pull/290)] @mcncl

### Fixed

-   Support TF version < 0.15.4 [[PR #294](https://github.com/buildkite/terraform-provider-buildkite/pull/294)] @mcncl

## [v0.19.1](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.19.0...v0.19.1)

### Added

-   SUP-202 Add graphql example queries for finding import IDs [[PR #280](https://github.com/buildkite/terraform-provider-buildkite/pull/280)] @james2791 @jradtilbrook
-   SUP-1072 Create new provider using framework plugin [[PR #286](https://github.com/buildkite/terraform-provider-buildkite/pull/286)] @jradtilbrook
-   SUP-1066 Migrate agent token to framework [[PR #289](https://github.com/buildkite/terraform-provider-buildkite/pull/289)] @jradtilbrook
-   Omit empty buildRetentionEnabled input [[PR #291](https://github.com/buildkite/terraform-provider-buildkite/pull/291)] @jradtilbrook

## [v0.19.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.18.0...v0.19.0)

### Breaking changes

This release removes the ability to import Agent tokens.  
As per https://buildkite.com/changelog/207-agent-token-being-deprecated-from-graphql-apis, it will soon not be possible
to read the agent token value after creation, making importing Agent tokens impossible.

**Note**: If you are using an earlier version than `v0.19.0` after the change above occurs, you will likely see an
unexpected diff in your `terraform plan`s. Upon state refresh, the token values will be emptied out which could trigger
other dependent resources to change. It is highly recommended to upgrade to `v0.19.0` prior to avoid this happening.

### Fixed

-   Pipelines resource Computed/Default nil values reversion [[PR #277](https://github.com/buildkite/terraform-provider-buildkite/pull/277)] @james2791
-   Allow pipeline to be removed from a cluster [[PR #279](https://github.com/buildkite/terraform-provider-buildkite/pull/279)] @jradtilbrook
-   Change default provider settings to match new pipeline [[PR #282](https://github.com/buildkite/terraform-provider-buildkite/pull/282)] @jradtilbrook

### Added

-   Agent Token resource genqlient migration & adjustment [[PR #281](https://github.com/buildkite/terraform-provider-buildkite/pull/281)] @james2791

## [v0.18.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.17.1...v0.18.0)

### Added

-   DOCS fmt [[PR #271](https://github.com/buildkite/terraform-provider-buildkite/pull/271)] @mattclegg
-   Add PR/issue templates [[PR #272](https://github.com/buildkite/terraform-provider-buildkite/pull/272)] @jradtilbrook
-   Add bug report issue template [[PR #273](https://github.com/buildkite/terraform-provider-buildkite/pull/273)] @jradtilbrook
-   Use gotestsum for running and reporting tests [[PR #274](https://github.com/buildkite/terraform-provider-buildkite/pull/274)] @jradtilbrook
-   Remove Computed values from Pipeline where not required [[PR #275](https://github.com/buildkite/terraform-provider-buildkite/pull/275)] @mcncl
-   SUP-995: Changelog updates, v0.18.0 release prep [[PR #276](https://github.com/buildkite/terraform-provider-buildkite/pull/276)] @james2791

### Fixed

-   Fixed issues when bumping genqlient from 0.4.0 to 0.6.0 [[PR #278](https://github.com/buildkite/terraform-provider-buildkite/pull/278)] @lizrabuya

## [v0.17.1](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.17.0...v0.17.1)

### Fixed

-   SUP-906/Fix: Adjustment of README version, Pipeline Argument Reference amendments [[PR #269](https://github.com/buildkite/terraform-provider-buildkite/pull/269)] @james2791

## [v0.17.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.16.0...v0.17.0)

### Added

-   SUP-707: Pipeline resource's steps made optional, default pipeline upload step [[PR #265](https://github.com/buildkite/terraform-provider-buildkite/pull/265)] @james2791
-   Update the paths where we load environment variables from [[PR #266](https://github.com/buildkite/terraform-provider-buildkite/pull/265)] @yob

## [v0.16.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.15.0...v0.16.0)

### Added

-   Allow release pipeline to pull from private ECR registry [[PR #258](https://github.com/buildkite/terraform-provider-buildkite/pull/258)] @ellsclytn
-   Set User-Agent header on both REST and graphql requests [[PR #259](https://github.com/buildkite/terraform-provider-buildkite/pull/259)] @yob
-   Update terraform SDKv2 to the latest version [[PR #261](https://github.com/buildkite/terraform-provider-buildkite/pull/261)] @yob
-   Run go mod tidy [[PR #262](https://github.com/buildkite/terraform-provider-buildkite/pull/262)] @yob
-   SUP-819 Retrieve org id only once in provider configuration [[PR #263](https://github.com/buildkite/terraform-provider-buildkite/pull/263)] @jradtilbrook
-   SUP-820 Pull latest 1.4 version terraform from docker image [[PR #264](https://github.com/buildkite/terraform-provider-buildkite/pull/264)] @jradtilbrook

## [v0.15.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.14.0...v0.15.0)

### Added

-   Don't include null body in GET requests [[PR #254](https://github.com/buildkite/terraform-provider-buildkite/pull/254)] @nhurden
-   Set User-Agent header in client [[PR #256](https://github.com/buildkite/terraform-provider-buildkite/pull/256)] @danstn

## [v0.14.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.12.2...v0.14.0)

### Added

-   Update Go and the package dependendies (inc SUP-841) [[PR #240](https://github.com/buildkite/terraform-provider-buildkite/pull/240)] @mcncl
-   SUP-857 Manually update go getter & deps [[PR #246](https://github.com/buildkite/terraform-provider-buildkite/pull/246)] @mcncl
-   Add documentation around plugin override for local dev [[PR #247](https://github.com/buildkite/terraform-provider-buildkite/pull/247)] @mcncl
-   SUP-838 Ensure CI passes/fails correctly [[PR #248](https://github.com/buildkite/terraform-provider-buildkite/pull/248)] @jradtilbrook
-   SUP-866 Add deletion protection for a pipeline [[PR #250](https://github.com/buildkite/terraform-provider-buildkite/pull/250)] @mcncl

### Fixed

-   SUP-770 Fix panic in `GetOrganizationID` [[PR #249](https://github.com/buildkite/terraform-provider-buildkite/pull/249)] @jradtilbrook

## [v0.12.2](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.12.1...v0.12.2)

### Added

-   Add a reviewer to dependabot PRs [[PR #236](https://github.com/buildkite/terraform-provider-buildkite/pull/236)] @yob
-   Add support for timeout settings. [[PR #238](https://github.com/buildkite/terraform-provider-buildkite/pull/238)] @mcncl

## [v0.12.1](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.12.0...v0.12.1)

### Added

-   Add `organization_settings` documentation [[PR #237](https://github.com/buildkite/terraform-provider-buildkite/pull/237)] @mcncl

## [v0.12.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.11.1...v0.12.0)

### Added

-   Added Organisational settings [[PR #230](https://github.com/buildkite/terraform-provider-buildkite/pull/230)] @mcncl

## [v0.11.1](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.11.0...v0.11.1)

-   Documentation had fallen out of sync due to lack of tag use. This change in the version includes a tag bump in order to merge documentation changes/improvements in to the provider page.

###

-   GraphQL demo with Khan/genqlient [[PR #205](https://github.com/buildkite/terraform-provider-buildkite/pull/205)] @jradtilbrook
-   Add `allow_rebuilds documentation`. Grammar. [[#PR 208](https://github.com/buildkite/terraform-provider-buildkite/pull/208)] @jmctune
-   SUP-191 Generate test coverage [[PR #206](https://github.com/buildkite/terraform-provider-buildkite/pull/206)] @jradtilbrook
-   Add arch command for agnostic support of architecture in build command [[PR #228](https://github.com/buildkite/terraform-provider-buildkite/pull/228)] @mcncl
-   Update version and tag so that documentation is merged in to provider [[PR #229](https://github.com/buildkite/terraform-provider-buildkite/pull/229)] @mcncl

## [v0.11.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.10.0...v0.11.0)

### Added

-   Allow configuring API endpoints [[PR #202](https://github.com/buildkite/terraform-provider-buildkite/pull/202)] @jradtilbrook

## [v0.10.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.9.0...v0.10.0)

### Added

-   Allow tests to run on PRs [[PR #197](https://github.com/buildkite/terraform-provider-buildkite/pull/197)] @jradtilbrook

### Fixed

-   Fix cluster support [[#PR 200](https://github.com/buildkite/terraform-provider-buildkite/pull/200)] @jradtilbrook

## [v0.9.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.8.0...v0.9.0)

### Added

-   Add team data source [[PR #190](https://github.com/buildkite/terraform-provider-buildkite/pull/190)] @margueritepd
-   add support for tags [[PR #186](https://github.com/buildkite/terraform-provider-buildkite/pull/186)] @mhornbacher
-   Add allow rebuilds to pipeline resource [[#PR 193](https://github.com/buildkite/terraform-provider-buildkite/pull/193)] @jradtilbrook

## [v0.8.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.7.0...v0.8.0)

### Added

-   Add `cluster_id` argument to pipeline resource [[PR #181](https://github.com/buildkite/terraform-provider-buildkite/pull/181)] @kate-syberspace

## [v0.7.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.6.0...v0.7.0)

### Fixed

-   Brought back build target for darwin/arm64 [[PR #189](https://github.com/buildkite/terraform-provider-buildkite/pull/189)] @mhornbacher

## [v0.6.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.5.0...v0.6.0)

### Added

-   `buildkite_team_member` resource: Manage organisation team membership [[PR #173](https://github.com/buildkite/terraform-provider-buildkite/pull/173)] @jradtilbrook
    Bump Golang to 1.17.3
-   Pipeline resource: Add build_pull_request_labels_changed property [[PR #164](https://github.com/buildkite/terraform-provider-buildkite/pull/164)] @hadusam

### Fixed

-   Fixed typo in pipeline docs [[PR #172](https://github.com/buildkite/terraform-provider-buildkite/pull/172)] @RussellRollins
-   Added `cancel_deleted_branch_builds` to pipeline docs [[PR #160](https://github.com/buildkite/terraform-provider-buildkite/pull/160)] @keith

## [v0.5.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.4.0...v0.5.0)

### Added

-   pipeline resource: Add badge_url property [[PR #151](https://github.com/buildkite/terraform-provider-buildkite/pull/151)] @JPScutt
-   pipeline resource: Add filter_condition and filter_enabled to provider_settings [[PR #157](https://github.com/buildkite/terraform-provider-buildkite/pull/157)] @gu-kevin

### Fixed

-   Improved documentation for pipeline resource [[PR #145](https://github.com/buildkite/terraform-provider-buildkite/pull/145)] [[PR #146](https://github.com/buildkite/terraform-provider-buildkite/pull/146)] @jlisee
-   Improved error when an unrecognised team slug is used in pipeline resource [[PR #155](https://github.com/buildkite/terraform-provider-buildkite/pull/155)] @yob
-   Improved error message when an unrecognised ID is used while importing a pipeline schedule [[PR #144](https://github.com/buildkite/terraform-provider-buildkite/pull/144)] @yob

## [v0.4.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.3.0...v0.4.0)

### Added

-   `buildkite_meta` data source for fetching the IP addresses Buildkite uses for webhooks [[PR #136](https://github.com/buildkite/terraform-provider-buildkite/pull/136)] @yob

## [v0.3.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.2.0...v0.3.0)

### Added

-   `buildkite_pipeline` resource can now manage provider settings (which webhooks to build on, etc) [[PR #123](https://github.com/buildkite/terraform-provider-buildkite/pull/123)] @vgrigoruk

### Changed

-   `buildkite_pipeline` resource: use 'Computed: true' for attributes that are initialized on backend [[PR #125](https://github.com/buildkite/terraform-provider-buildkite/pull/125)] @vgrigoruk
    -   when these properties are unspecified in terraform, default values are left to Buildkite to define

## [v0.2.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.1.0...v0.2.0)

### Added

-   New `buildkite_pipeline` data source [[PR #106](https://github.com/buildkite/terraform-provider-buildkite/pull/106)] @yob

### Changed

-   Add darwin/arm64 (M1 systems) to the build matrix [[PR #104](https://github.com/buildkite/terraform-provider-buildkite/pull/104)] @yob
-   The following resources and data sources can now be used by API tokens that belong to non administrators, provided
    the token belongs to a user who has team maintainer permissions [[PR #112](https://github.com/buildkite/terraform-provider-buildkite/pull/112)] @chloeruka @yob
    -   `buildkite_pipeline` resource
    -   `buildkite_pipeline` data source
    -   `buildkite_pipeline_schedule` resource
-   All resources and data sources now have acceptance tests [many PRs] @chloeruka @yob

## [v0.1.0](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.0.17...v0.1.0)

### Added

-   New `pipeline_schedule` resource [[PR #87](https://github.com/buildkite/terraform-provider-buildkite/pull/87)] @vgrigoruk

### Changed

-   Require terraform 0.13 or greater [[PR #89](https://github.com/buildkite/terraform-provider-buildkite/pull/89)] @vgrigoruk
-   Add PowerPC 64 LE to the build matrix [[PR #92](https://github.com/buildkite/terraform-provider-buildkite/pull/92)] @runlevel5

## [v0.0.17](https://github.com/buildkite/terraform-provider-buildkite/compare/v0.0.16...v0.0.17)

-   No code changes from 0.0.16 - just the first release signed by a buildkite gpg key that will be
    available in the terraform registry as buildkite/buildkite.
