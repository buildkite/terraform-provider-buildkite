package buildkite

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/shurcooL/graphql"
)

func TestAccNonAdminOrganizationRuleCreate(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}
	testAccNonAdminPreCheck(t)
	uuid := testAccFixtureReadableRuleUUID
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	response, err := getOrganizationRule(ctx, genqlientGraphql, uuid)
	if err != nil {
		t.Fatalf("read Rule fixture %s: %v", uuid, err)
	}
	fixture := response.Rule
	source, sourceOK := fixture.Source.(*OrganizationRuleFieldsSourcePipeline)
	target, targetOK := fixture.Target.(*OrganizationRuleFieldsTargetPipeline)
	if fixture.Id == "" || fixture.Uuid != uuid || fixture.Type != "pipeline.artifacts_read.pipeline" ||
		!sourceOK || !targetOK || source.Uuid == "" || target.Uuid == "" || source.Uuid == target.Uuid {
		t.Fatal("Rule fixture must be a readable artifacts-read Rule between two distinct pipelines; check fixture access and member Rules-read enrollment")
	}

	// Rule uniqueness excludes conditions and description. Keep the trigger
	// action unused so the admin lane proves this create configuration is valid.
	checkTriggerAbsent := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var query struct {
			Organization struct {
				Rules struct {
					Edges []struct {
						Node struct {
							ID     graphql.ID
							Action graphql.String
						}
					}
					PageInfo struct{ HasNextPage graphql.Boolean }
				} `graphql:"rules(first: 100, sourceId: $source, targetId: $target)"`
			} `graphql:"organization(slug: $slug)"`
		}
		variables := map[string]any{
			"slug":   graphql.ID(getenv("BUILDKITE_ORGANIZATION_SLUG")),
			"source": graphql.ID(base64.StdEncoding.EncodeToString([]byte("Pipeline---" + source.Uuid))),
			"target": graphql.ID(base64.StdEncoding.EncodeToString([]byte("Pipeline---" + target.Uuid))),
		}
		if err := graphqlClient.Query(ctx, &query, variables); err != nil {
			return fmt.Errorf("verify Rule fixture pair: %w", err)
		}
		if query.Organization.Rules.PageInfo.HasNextPage {
			return fmt.Errorf("Rule fixture pair query was truncated; cannot verify trigger Rule absence")
		}
		foundFixture := false
		for _, edge := range query.Organization.Rules.Edges {
			if edge.Node.ID == fixture.Id {
				foundFixture = true
			}
			if edge.Node.Action == "TRIGGER_BUILD" {
				return fmt.Errorf("unexpected trigger Rule %v between fixture pipelines; an administrator must inspect and remove it before rerunning", edge.Node.ID)
			}
		}
		if !foundFixture {
			return fmt.Errorf("persistent Rule %s is missing from the endpoint-filtered list; cannot verify trigger Rule absence", uuid)
		}
		return nil
	}
	if err := checkTriggerAbsent(); err != nil {
		t.Fatal(err)
	}
	// Also inspect remote state if the expected denial fails. Never delete a
	// leaked Rule using fallback admin credentials or touch persistent read Rules.
	t.Cleanup(func() {
		if err := checkTriggerAbsent(); err != nil {
			t.Error(err)
		}
	})

	marker := acctest.RandString(12)
	config := fmt.Sprintf(`
		provider "buildkite" {
			timeouts = { create = "30s", read = "30s", delete = "30s" }
		}
		data "buildkite_organization_rule" "fixture" {
			uuid = %q
		}`, uuid)
	create := resource.TestStep{
		Config: config + fmt.Sprintf(`
			resource "buildkite_organization_rule" "temporary" {
				type = "pipeline.trigger_build.pipeline"
				description = "Terraform acceptance Rule create %s"
				value = jsonencode({
					source_pipeline = data.buildkite_organization_rule.fixture.source_uuid
					target_pipeline = data.buildkite_organization_rule.fixture.target_uuid
					conditions = ["source.build.branch == 'terraform-provider-rule-create-%s'"]
				})
			}`, marker, marker),
	}
	if os.Getenv("BUILDKITE_ACCEPTANCE_NONADMIN") == "1" {
		create.ExpectError = regexp.MustCompile("(?s)Unable to create organization rule.*You’re not allowed")
	} else {
		create.Check = resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.temporary", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.temporary", "uuid"),
			resource.TestCheckResourceAttr("buildkite_organization_rule.temporary", "type", "pipeline.trigger_build.pipeline"),
			resource.TestCheckResourceAttr("buildkite_organization_rule.temporary", "description", "Terraform acceptance Rule create "+marker),
			resource.TestCheckResourceAttr("buildkite_organization_rule.temporary", "source_uuid", source.Uuid),
			resource.TestCheckResourceAttr("buildkite_organization_rule.temporary", "target_uuid", target.Uuid),
		)
	}
	resource.ParallelTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("data.buildkite_organization_rule.fixture", "id", fixture.Id),
			},
			create,
		},
	})
}

func TestAccBuildkiteOrganizationRuleMutationPermissions(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}
	testAccPreCheck(t)
	memberToken := os.Getenv("BUILDKITE_NONADMIN_API_TOKEN")
	if memberToken == "" {
		t.Fatal("BUILDKITE_NONADMIN_API_TOKEN must contain the member token for Rule update/delete denial tests; BUILDKITE_API_TOKEN must remain the admin token")
	}
	testAccMemberPreCheck(t, getTestClientWithToken(memberToken).graphql)
	fixtureUUID := testAccFixtureReadableRuleUUID
	// Keep this test serial: it temporarily occupies the same trigger slot as
	// TestAccNonAdminOrganizationRuleCreate. No credentials are changed globally.
	t.Setenv("TF_VAR_rule_member_token", memberToken)
	marker := "terraform-provider-rule-mutations-" + acctest.RandString(12)
	fixtureConfig := fmt.Sprintf(`
		provider "buildkite" {
			timeouts = { create = "30s", read = "30s", update = "30s", delete = "30s" }
		}
		data "buildkite_organization_rule" "fixture" {
			uuid = %q
		}`, fixtureUUID)
	ruleConfig := func(revision string) string {
		return fmt.Sprintf(`
			resource "buildkite_organization_rule" "temporary" {
				type = "pipeline.trigger_build.pipeline"
				description = %q
				value = jsonencode({
					source_pipeline = data.buildkite_organization_rule.fixture.source_uuid
					target_pipeline = data.buildkite_organization_rule.fixture.target_uuid
					conditions = [%q]
				})
			}`, marker+"-"+revision, "source.build.branch == '"+marker+"-"+revision+"'")
	}
	var ruleID string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckOrganizationRuleDestroy,
		Steps: []resource.TestStep{
			{
				Config: fixtureConfig + ruleConfig("original"),
				Check: func(s *terraform.State) error {
					ruleID = s.RootModule().Resources["buildkite_organization_rule.temporary"].Primary.ID
					return resource.TestCheckResourceAttrSet("buildkite_organization_rule.temporary", "uuid")(s)
				},
			},
			{
				Config: fixtureConfig + ruleConfig("updated"),
				Check: func(s *terraform.State) error {
					return resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_organization_rule.temporary", "id", ruleID),
						resource.TestCheckResourceAttr("buildkite_organization_rule.temporary", "description", marker+"-updated"),
					)(s)
				},
			},
			{
				Config: fixtureConfig + ruleConfig("original"),
				Check: func(s *terraform.State) error {
					attrs := s.RootModule().Resources["buildkite_organization_rule.temporary"].Primary.Attributes
					memberConfig := fmt.Sprintf(`
						variable "rule_member_token" {
							type = string
							sensitive = true
						}
						provider "buildkite" {
							api_token = var.rule_member_token
							timeouts = { read = "30s", update = "30s", delete = "30s" }
						}
						data "buildkite_organization_rule" "fixture" {
							uuid = %q
						}`, fixtureUUID)
					for _, operation := range []string{"update", "delete"} {
						t.Run("member_"+operation, func(t *testing.T) {
							attempt := resource.TestStep{
								Config:      memberConfig + ruleConfig("updated"),
								ExpectError: regexp.MustCompile("(?s)Unable to " + operation + " organization rule.*You’re not allowed"),
							}
							if operation == "delete" {
								attempt.Config = memberConfig + ruleConfig("original")
								attempt.Destroy = true
							}
							// Forget only the member's imported state. The independent
							// admin state owns the disposable Rule and always destroys it,
							// even if a member subtest fails before reaching this step.
							verifyConfig := memberConfig + fmt.Sprintf(`
								removed {
									from = buildkite_organization_rule.temporary
									lifecycle { destroy = false }
								}
								data "buildkite_organization_rule" "unchanged" {
									uuid = %q
								}`, attrs["uuid"])
							var checks []resource.TestCheckFunc
							for _, field := range []string{"id", "uuid", "type", "description", "value", "source_uuid", "target_uuid", "action", "effect"} {
								checks = append(checks, resource.TestCheckResourceAttr("data.buildkite_organization_rule.unchanged", field, attrs[field]))
							}
							resource.Test(t, resource.TestCase{
								ProtoV6ProviderFactories: protoV6ProviderFactories(),
								Steps: []resource.TestStep{
									{
										Config:             memberConfig + ruleConfig("original"),
										ResourceName:       "buildkite_organization_rule.temporary",
										ImportState:        true,
										ImportStateId:      ruleID,
										ImportStatePersist: true,
									},
									attempt,
									{
										Config: verifyConfig,
										Check:  resource.ComposeAggregateTestCheckFunc(checks...),
									},
								},
							})
						})
					}
					return nil
				},
			},
		},
	})
}

func TestAccBuildkiteOrganizationRuleResource(t *testing.T) {
	t.Parallel()

	ruleActions := []string{"trigger_build", "artifacts_read"}

	configRequired := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2])
	}

	configAll := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	configAllCustomDescription := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule with a custom %s description"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2], fields[3])
	}

	configAllWithoutDescription := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2])
	}

	configAllCustomConditions := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule with a custom description"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [%s]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2], fields[3])
	}

	configRequiredRuleTypeChange := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "type_replace" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2])
	}

	configRequiredNewSource := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_new_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_new_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
			})
		}

		`, fields[0], fields[1], fields[0], fields[2], fields[1], fields[3], fields[3])
	}

	configRequiredNewTarget := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}

		resource "buildkite_pipeline" "pipeline_new_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_new_target.uuid
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[3], fields[3])
	}

	configRequiredSwap := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_target.uuid
				target_pipeline = buildkite_pipeline.pipeline_source.uuid
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2])
	}

	configNonExistentAction := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}	

		resource "buildkite_organization_rule" "non_existent_action_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.non_existent_action.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_target.uuid
				target_pipeline = buildkite_pipeline.pipeline_source.uuid
			})
		}

		`, fields[0], fields[1])
	}

	configInvalidConditional := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}	

		resource "buildkite_organization_rule" "non_existent_condition" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_target.uuid
				target_pipeline = buildkite_pipeline.pipeline_source.uuid
				conditions = [
					"source.nonexistent_condition includes 'idontexist'"
				]
			})
		}

		`, fields[0], fields[1])
	}

	configNoSourcePipelineUUID := func(targetName string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}	

		resource "buildkite_organization_rule" "no_source_pipeline_rule" {
			depends_on = [buildkite_pipeline.pipeline_target]
			type = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
			})
		}

		`, targetName)
	}

	configNoTargetPipelineUUID := func(sourceName string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}

		resource "buildkite_organization_rule" "no_target_pipeline_rule" {
			depends_on = [buildkite_pipeline.pipeline_source]
			type = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
			})
		}

		`, sourceName)
	}

	configSourceUUIDInvalid := func(targetName string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}	

		resource "buildkite_organization_rule" "rule_without_a_source" {
			depends_on = [buildkite_pipeline.pipeline_target]
			type = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				source_pipeline = "non_existent"
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
			})
		}

		`, targetName)
	}

	configTargetUUIDInvalid := func(sourceName string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
		}	

		resource "buildkite_organization_rule" "rule_without_a_target" {
			depends_on = [buildkite_pipeline.pipeline_source]
			type = "pipeline.trigger_build.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = "non-existent"
			})
		}

		`, sourceName)
	}

	configUpdateErrorInvalidSource := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule description"
			value = jsonencode({
				source_pipeline = "non-existent"
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	configUpdateErrorInvalidTarget := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule description"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_target.uuid
				target_pipeline = "non-existent"
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	configUpdateErrorNoSource := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule description"
			value = jsonencode({
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	configUpdateErrorNoTarget := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule description"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	configUpdateErrorSourceKey := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule description"
			value = jsonencode({
				sourc_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	configUpdateErrorTargetKey := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			description = "A pipeline.%s.pipeline rule description"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipelin = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2])
	}

	for _, action := range ruleActions {
		// Formatted resource name used for all create/update/import test cases
		resourceName := fmt.Sprintf("buildkite_organization_rule.%s_rule", action)

		t.Run(fmt.Sprintf("creates a pipeline.%s.pipeline organization rule with required attributes", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
				// Assert organization rule resource's state values
				resource.TestCheckResourceAttr(resourceName, "type", fmt.Sprintf("pipeline.%s.pipeline", action)),
				resource.TestCheckResourceAttr(resourceName, "source_type", "PIPELINE"),
				resource.TestCheckResourceAttr(resourceName, "target_type", "PIPELINE"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  check,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("creates a pipeline.%s.pipeline organization rule with all attributes", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
				// Assert organization rule resource's state values
				resource.TestCheckResourceAttr(resourceName, "type", fmt.Sprintf("pipeline.%s.pipeline", action)),
				resource.TestCheckResourceAttr(resourceName, "description", fmt.Sprintf("A pipeline.%s.pipeline rule", action)),
				resource.TestCheckResourceAttr(resourceName, "source_type", "PIPELINE"),
				resource.TestCheckResourceAttr(resourceName, "target_type", "PIPELINE"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAll(randNameOne, randNameTwo, action),
						Check:  check,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by adding a description", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			description := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
				// Assert organization rule resource's state values
				resource.TestCheckResourceAttr(resourceName, "type", fmt.Sprintf("pipeline.%s.pipeline", action)),
				resource.TestCheckResourceAttr(resourceName, "source_type", "PIPELINE"),
				resource.TestCheckResourceAttr(resourceName, "target_type", "PIPELINE"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's description attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				// Asserts organization rule resource's added description is in state
				resource.TestCheckResourceAttr(resourceName, "description", fmt.Sprintf("A pipeline.%s.pipeline rule with a custom %s description", action, description)),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllWithoutDescription(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						Config: configAllCustomDescription(randNameOne, randNameTwo, action, description),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by editing its current description", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			description := acctest.RandString(12)
			updatedDescription := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
				// Asserts organization rule resource's initial description in state
				resource.TestCheckResourceAttr(resourceName, "description", fmt.Sprintf("A pipeline.%s.pipeline rule with a custom %s description", action, description)),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's description attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				// Asserts organization rule resource's updated description in state
				resource.TestCheckResourceAttr(resourceName, "description", fmt.Sprintf("A pipeline.%s.pipeline rule with a custom %s description", action, updatedDescription)),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllCustomDescription(randNameOne, randNameTwo, action, description),
						Check:  check,
					},
					{
						Config: configAllCustomDescription(randNameOne, randNameTwo, action, updatedDescription),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by removing its description", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			description := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
				// Check the organization rule resource's source, target, type and existing description attributes are set in state
				resource.TestCheckResourceAttr(resourceName, "type", fmt.Sprintf("pipeline.%s.pipeline", action)),
				resource.TestCheckResourceAttr(resourceName, "source_type", "PIPELINE"),
				resource.TestCheckResourceAttr(resourceName, "target_type", "PIPELINE"),
				resource.TestCheckResourceAttr(resourceName, "description", fmt.Sprintf("A pipeline.%s.pipeline rule with a custom %s description", action, description)),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllCustomDescription(randNameOne, randNameTwo, action, description),
						Check:  check,
					},
					{
						Config: configAllWithoutDescription(randNameOne, randNameTwo, action),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by changing its source_pipeline", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			randNameThree := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's source(s) and value attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						Config: configRequiredNewSource(randNameOne, randNameTwo, randNameThree, action),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by changing its target_pipeline", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			randNameThree := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's target(s) and value attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						Config: configRequiredNewTarget(randNameOne, randNameTwo, randNameThree, action),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by changing both source_pipeline and target_pipeline", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's source, target and value attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						Config: configRequiredSwap(randNameOne, randNameTwo, action),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by adding conditions", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			conditions := `
      			"source.build.branch == 'develop'",
      			"source.pipeline.slug == 'monorepo-core'",
				"source.build.creator.teams includes 'monorepo'"
			`

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's value attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, conditions),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by inserting additional conditions", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			initConditions := `
      			"source.build.branch == 'develop'",
      			"source.pipeline.slug == 'monorepo-core'"
			`

			updatedConditions := `
      			"source.build.branch == 'develop'",
      			"source.pipeline.slug == 'monorepo-core'",
				"source.build.creator.teams includes 'monorepo'"
			`

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's value attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, initConditions),
						Check:  check,
					},
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, updatedConditions),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by removing some conditions", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			initConditions := `
  				"source.build.branch == 'develop'",
      			"source.pipeline.slug == 'monorepo-core'",
				"source.build.creator.teams includes 'monorepo'"
			`

			updatedConditions := `
          		"source.build.branch == 'develop'",
				"source.build.creator.teams includes 'monorepo'"
			`

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's value attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, initConditions),
						Check:  check,
					},
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, updatedConditions),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by removing all conditions", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			existingConditions := `
      			"source.build.branch == 'develop'",
      			"source.pipeline.slug == 'monorepo-core'",
				"source.build.creator.teams includes 'monorepo'"
			`

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's value attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, existingConditions),
						Check:  check,
					},
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("updates a pipeline.%s.pipeline organization rule by editing its current conditions", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			initConditions := `
  				"source.build.branch == 'develop'",
      			"source.pipeline.slug == 'monorepo-core'",
				"source.build.creator.teams includes 'monorepo'"
			`

			updatedConditions := `
  				"source.build.branch == 'dev'",
      			"source.pipeline.slug == 'monorepo'",
				"source.build.creator.teams includes 'monorepo'"
			`

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			checkUpdated := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's value attribute is set in state
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, initConditions),
						Check:  check,
					},
					{
						Config: configAllCustomConditions(randNameOne, randNameTwo, action, updatedConditions),
						Check:  checkUpdated,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("imports a pipeline.%s.pipeline organization rule with required attributes", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequired(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						ResourceName:      resourceName,
						ImportState:       true,
						ImportStateVerify: true,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("imports a pipeline.%s.pipeline organization rule with all attributes", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "description"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAll(randNameOne, randNameTwo, action),
						Check:  check,
					},
					{
						ResourceName:      resourceName,
						ImportState:       true,
						ImportStateVerify: true,
					},
				},
			})
		})
	}

	t.Run("replaces an orgnanization rule when updating its type", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the organization rule exists
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.type_replace"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "value"),
			// Assert organization rule resource's state values
			resource.TestCheckResourceAttr("buildkite_organization_rule.type_replace", "type", "pipeline.trigger_build.pipeline"),
		)

		checkUpdated := resource.ComposeAggregateTestCheckFunc(
			// Confirm the organization rule exists
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.type_replace"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "ARTIFACTS_READ", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.type_replace", "value"),
			// Assert organization rule resource's state values
			resource.TestCheckResourceAttr("buildkite_organization_rule.type_replace", "type", "pipeline.artifacts_read.pipeline"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configRequiredRuleTypeChange(randNameOne, randNameTwo, "trigger_build"),
					Check:  check,
				},
				{
					Config: configRequiredRuleTypeChange(randNameOne, randNameTwo, "artifacts_read"),
					Check:  checkUpdated,
				},
			},
		})
	})

	t.Run("errors when an organization rule is created with an unknown action", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configNonExistentAction(randNameOne, randNameTwo),
					ExpectError: regexp.MustCompile("input: Rule type is unknown"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is created with an invalid conditional", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configInvalidConditional(randNameOne, randNameTwo),
					ExpectError: regexp.MustCompile("conditional is invalid:\n`source.nonexistent_condition` is not a variable"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is created with a missing source_pipeline in its value", func(t *testing.T) {
		randName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configNoSourcePipelineUUID(randName),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing source_pipeline"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is created with a missing target_pipeline in its value", func(t *testing.T) {
		randName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configNoTargetPipelineUUID(randName),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing target_pipeline"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is created with an invalid source_pipeline slug", func(t *testing.T) {
		randName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configSourceUUIDInvalid(randName),
					ExpectError: regexp.MustCompile("Unable to resolve pipeline slug"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is created with an invalid target_pipeline slug", func(t *testing.T) {
		randName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configTargetUUIDInvalid(randName),
					ExpectError: regexp.MustCompile("Unable to resolve pipeline slug"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with a malformed source_pipeline key", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randNameOne, randNameTwo, "trigger_build"),
					Check:  check,
				},
				{
					Config:      configUpdateErrorSourceKey(randNameOne, randNameTwo, "trigger_build"),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing source_pipeline"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with a malformed target_pipeline key", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randNameOne, randNameTwo, "trigger_build"),
					Check:  check,
				},
				{
					Config:      configUpdateErrorTargetKey(randNameOne, randNameTwo, "trigger_build"),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing target_pipeline"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with an invalid source_pipeline UUID", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randNameOne, randNameTwo, "trigger_build"),
					Check:  check,
				},
				{
					Config:      configUpdateErrorInvalidSource(randNameOne, randNameTwo, "trigger_build"),
					ExpectError: regexp.MustCompile("Unable to resolve pipeline slug"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with an invalid target_pipeline UUID", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randNameOne, randNameTwo, "trigger_build"),
					Check:  check,
				},
				{
					Config:      configUpdateErrorInvalidTarget(randNameOne, randNameTwo, "trigger_build"),
					ExpectError: regexp.MustCompile("Unable to resolve pipeline slug"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with no source_pipeline UUID", func(t *testing.T) {
		ruleType := "trigger_build"
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randNameOne, randNameTwo, ruleType),
					Check:  check,
				},
				{
					Config:      configUpdateErrorNoSource(randNameOne, randNameTwo, ruleType),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing source_pipeline"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with no target_pipeline UUID", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAll(randNameOne, randNameTwo, "trigger_build"),
					Check:  check,
				},
				{
					Config:      configUpdateErrorNoTarget(randNameOne, randNameTwo, "trigger_build"),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: missing target_pipeline"),
				},
			},
		})
	})

	t.Run("errors when an organization rule is updated with invalid conditions", func(t *testing.T) {
		randNameOne := acctest.RandString(12)
		randNameTwo := acctest.RandString(12)
		var orr organizationRuleResourceModel

		initConditions := `
			"source.build.branch == 'develop'",
			"source.pipeline.slug == 'monorepo-core'"
  		`

		nonExistentConditions := `
			"source.build.branch == 'develop'",
			"source.pipeline.slug == 'monorepo-core'",
			"source.build.polisher includes 'monorepo'"
		`

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckOrganizationRuleExists(&orr, "buildkite_organization_rule.trigger_build_rule"),
			// Confirm the organization rule has the correct values in Buildkite's system
			testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", "TRIGGER_BUILD", "ALLOW"),
			// Check the organization rule resource's attributes are set in state
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "id"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "description"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "source_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_type"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "target_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_organization_rule.trigger_build_rule", "value"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckOrganizationRuleDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAllCustomConditions(randNameOne, randNameTwo, "trigger_build", initConditions),
					Check:  check,
				},
				{
					Config:      configAllCustomConditions(randNameOne, randNameTwo, "trigger_build", nonExistentConditions),
					ExpectError: regexp.MustCompile("pipeline.trigger_build.pipeline: conditional is invalid"),
				},
			},
		})
	})

	configSlug := func(sourceName, targetName, action string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_source" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_target" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			depends_on 			 = [buildkite_cluster.cluster_source]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_source.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			depends_on 			 = [buildkite_cluster.cluster_target]
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_target.id
		}

		resource "buildkite_organization_rule" "%s_rule" {
			depends_on = [
				buildkite_pipeline.pipeline_source,
				buildkite_pipeline.pipeline_target
			]
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = "${buildkite_pipeline.pipeline_source.slug}"
				target_pipeline = "${buildkite_pipeline.pipeline_target.slug}"
			})
		}

		`, sourceName, targetName, sourceName, targetName, action, action)
	}

	for _, action := range ruleActions {
		// Formatted resource name used for slug-based test cases
		resourceName := fmt.Sprintf("buildkite_organization_rule.%s_rule", action)

		t.Run(fmt.Sprintf("creates a pipeline.%s.pipeline organization rule using pipeline slugs", action), func(t *testing.T) {
			randNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, resourceName),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule resource's attributes are set in state
				resource.TestCheckResourceAttrSet(resourceName, "id"),
				resource.TestCheckResourceAttrSet(resourceName, "uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_type"),
				resource.TestCheckResourceAttrSet(resourceName, "source_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "target_type"),
				resource.TestCheckResourceAttrSet(resourceName, "target_uuid"),
				resource.TestCheckResourceAttrSet(resourceName, "value"),
				// Assert organization rule resource's state values
				resource.TestCheckResourceAttr(resourceName, "type", fmt.Sprintf("pipeline.%s.pipeline", action)),
				resource.TestCheckResourceAttr(resourceName, "source_type", "PIPELINE"),
				resource.TestCheckResourceAttr(resourceName, "target_type", "PIPELINE"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configSlug(randNameOne, randNameTwo, action),
						Check:  check,
					},
					// Verifies no plan diff on subsequent plan (idempotency check).
					{
						Config:   configSlug(randNameOne, randNameTwo, action),
						PlanOnly: true,
					},
				},
			})
		})
	}
}

func testAccCheckOrganizationRuleExists(orr *organizationRuleResourceModel, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]

		if !ok {
			return fmt.Errorf("Not found in state: %s", name)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}
		r, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)
		if err != nil {
			return err
		}

		if organizationRule, ok := r.GetNode().(*getNodeNodeRule); ok {
			if organizationRule == nil {
				return fmt.Errorf("Organization rule not found: nil response")
			}
			value, err := obtainValueJSON(organizationRule.Document)
			if err != nil {
				return fmt.Errorf("Error constructing sorted value JSON to store in state")
			}
			updateOrganizationRuleReadState(orr, *organizationRule, *value)
		}

		return nil
	}
}

func testAccCheckOrganizationRuleRemoteValues(orr *organizationRuleResourceModel, sourceType, targetType, action, effect string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if orr.SourceType.ValueString() != sourceType {
			return fmt.Errorf("Remote organization rule source type (%s) doesn't match expected value (%s)", orr.SourceType, sourceType)
		}

		if orr.TargetType.ValueString() != targetType {
			return fmt.Errorf("Remote organization rule target type (%s) doesn't match expected value (%s)", orr.TargetType, targetType)
		}

		if orr.Action.ValueString() != action {
			return fmt.Errorf("Remote organization rule action (%s) doesn't match expected value (%s)", orr.Action, action)
		}

		if orr.Effect.ValueString() != effect {
			return fmt.Errorf("Remote organization rule effect (%s) doesn't match expected value (%s)", orr.Effect, effect)
		}

		return nil
	}
}

func testAccCheckOrganizationRuleDestroy(s *terraform.State) error {
	for address, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_organization_rule" || strings.HasPrefix(address, "data.") {
			continue
		}

		r, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)
		if err != nil {
			if isResourceNotFoundError(err) {
				continue
			}
			return fmt.Errorf("error checking if organization rule exists: %v", err)
		}

		if organizationRule, ok := r.GetNode().(*getNodeNodeRule); ok {
			if organizationRule != nil {
				return fmt.Errorf("Organization rule still exists")
			}
		}

	}
	return nil
}
