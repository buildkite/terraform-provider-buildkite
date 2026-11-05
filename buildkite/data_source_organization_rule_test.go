package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccNonAdminOrganizationRuleReads(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}
	testAccNonAdminPreCheck(t)
	uuid := os.Getenv("BUILDKITE_NONADMIN_RULE_UUID")
	if uuid == "" {
		t.Fatal("BUILDKITE_NONADMIN_RULE_UUID must identify an existing Rule visible to the member token; provision the fixture before running acceptance tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	remote, err := getOrganizationRule(ctx, genqlientGraphql, uuid)
	if err != nil {
		t.Fatalf("read Rule fixture %s: %v", uuid, err)
	}
	rule := remote.Rule
	if rule.Id == "" || rule.Uuid != uuid {
		t.Fatalf("Rule fixture %s is not visible: check the UUID, access to both pipelines, and member Rules-read feature enrollment", uuid)
	}
	source, sourceOK := rule.Source.(*OrganizationRuleFieldsSourcePipeline)
	target, targetOK := rule.Target.(*OrganizationRuleFieldsTargetPipeline)
	if !sourceOK || !targetOK || source.Uuid == "" || target.Uuid == "" || source.Uuid == target.Uuid {
		t.Fatal("Rule fixture must reference two distinct accessible pipelines")
	}
	var document struct {
		Value map[string]any `json:"value"`
	}
	if err := json.Unmarshal([]byte(rule.Document), &document); err != nil || document.Value == nil {
		t.Fatalf("Rule fixture has an invalid document: %s", rule.Document)
	}
	checkValue := func(value string) error {
		var got map[string]any
		if err := json.Unmarshal([]byte(value), &got); err != nil {
			return err
		}
		if !reflect.DeepEqual(got, document.Value) {
			return fmt.Errorf("Rule value = %s, want API document value %v", value, document.Value)
		}
		return nil
	}
	attrs := map[string]string{
		"id": rule.Id, "uuid": uuid, "type": rule.Type,
		"source_type": "PIPELINE", "source_uuid": source.Uuid,
		"target_type": "PIPELINE", "target_uuid": target.Uuid,
		"action": string(rule.Action), "effect": string(rule.Effect),
	}
	if rule.Description != nil {
		attrs["description"] = *rule.Description
	}
	var checks []resource.TestCheckFunc
	for _, name := range []string{"by_uuid", "by_id"} {
		address := "data.buildkite_organization_rule." + name
		for attr, want := range attrs {
			checks = append(checks, resource.TestCheckResourceAttr(address, attr, want))
		}
		checks = append(checks, resource.TestCheckResourceAttrWith(address, "value", checkValue))
		if rule.Description == nil {
			checks = append(checks, resource.TestCheckNoResourceAttr(address, "description"))
		}
	}
	value, err := json.Marshal(document.Value)
	if err != nil {
		t.Fatal(err)
	}

	resource.ParallelTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					data "buildkite_organization_rule" "by_uuid" {
						uuid = %q
					}
					data "buildkite_organization_rule" "by_id" {
						id = %q
					}`, uuid, rule.Id),
				Check: resource.ComposeAggregateTestCheckFunc(checks...),
			},
			{
				Config: fmt.Sprintf(`
					resource "buildkite_organization_rule" "fixture" {
						type = %q
						value = %q
					}`, rule.Type, string(value)),
				ResourceName:  "buildkite_organization_rule.fixture",
				ImportState:   true,
				ImportStateId: rule.Id,
				// Use a temporary import workspace, never the state destroyed by
				// acceptance cleanup: this externally managed Rule is read-only.
				ImportStatePersist: false,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("imported %d resources, want one Rule", len(states))
					}
					for attr, want := range attrs {
						if got := states[0].Attributes[attr]; got != want {
							return fmt.Errorf("imported %s = %q, want %q", attr, got, want)
						}
					}
					return checkValue(states[0].Attributes["value"])
				},
			},
		},
	})
}

func TestOrganizationRuleDatasourceReadErrors(t *testing.T) {
	for _, lookup := range []string{"id", "uuid"} {
		t.Run(lookup, func(t *testing.T) {
			for _, tc := range []struct {
				name     string
				response string
				message  string
			}{
				{
					name:     "inaccessible",
					response: `{"data":{"node":null,"rule":null}}`,
					message:  "Organization rule not found or not accessible",
				},
				{
					name:     "api_error",
					response: `{"errors":[{"message":"You're not allowed to do that"}]}`,
					message:  "You're not allowed to do that",
				},
				{
					name:     "malformed_document",
					response: `{"data":{"node":{"__typename":"Rule","id":"rule-id","document":"invalid-json"},"rule":{"id":"rule-id","document":"invalid-json"}}}`,
					message:  "error unmarshalling the organization rule's",
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprint(w, tc.response)
					}))
					defer server.Close()

					resource.UnitTest(t, resource.TestCase{
						ProtoV6ProviderFactories: protoV6ProviderFactories(),
						Steps: []resource.TestStep{
							{
								Config: fmt.Sprintf(`
									provider "buildkite" {
										organization = "test-org"
										api_token = "test-token"
										graphql_url = %q
										max_retries = 0
										timeouts = { read = "2s" }
									}
									data "buildkite_organization_rule" "rule" {
										%s = "inaccessible-rule"
									}`, server.URL, lookup),
								ExpectError: regexp.MustCompile(regexp.QuoteMeta(tc.message)),
							},
						},
					})
				})
			}
		})
	}
}

func TestAccBuildkiteOrganizationRuleDatasource(t *testing.T) {
	t.Parallel()

	ruleActions := []string{"trigger_build", "artifacts_read"}

	configRequiredByID := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_one" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_two" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_one.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_two.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
			})
		}

		data "buildkite_organization_rule" "%s_rule" {
			depends_on = [buildkite_organization_rule.%s_rule]
			id = buildkite_organization_rule.%s_rule.id
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2], fields[2], fields[2])
	}

	configRequiredByUUID := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_one" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_two" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_one.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_two.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			type = "pipeline.%s.pipeline"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
			})
		}

		data "buildkite_organization_rule" "%s_rule" {
			depends_on = [buildkite_organization_rule.%s_rule]
			uuid = buildkite_organization_rule.%s_rule.uuid
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2], fields[2], fields[2])
	}

	configAllByID := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_one" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_two" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_one.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_two.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			type = "pipeline.%s.pipeline"
			description = "A %s organization rule"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		data "buildkite_organization_rule" "%s_rule" {
			depends_on = [buildkite_organization_rule.%s_rule]
			id = buildkite_organization_rule.%s_rule.id
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2], fields[2], fields[2], fields[2])
	}

	configAllByUUID := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_one" {
			name        = "Cluster %s"
			description = "A test cluster containing a source pipeline."
		}

		resource "buildkite_cluster" "cluster_two" {
			name        = "Cluster %s"
			description = "A test cluster containing a target pipeline for triggering builds."
		}

		resource "buildkite_pipeline" "pipeline_source" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id			 = buildkite_cluster.cluster_one.id
		}

		resource "buildkite_pipeline" "pipeline_target" {
			name                 = "Pipeline %s"
			repository           = "https://github.com/buildkite/terraform-provider-buildkite.git"
			cluster_id           = buildkite_cluster.cluster_two.id
		}	

		resource "buildkite_organization_rule" "%s_rule" {
			type = "pipeline.%s.pipeline"
			description = "A %s organization rule"
			value = jsonencode({
				source_pipeline = buildkite_pipeline.pipeline_source.uuid
				target_pipeline = buildkite_pipeline.pipeline_target.uuid
				conditions = [
					"source.build.creator.teams includes 'deploy'",
					"source.build.branch == 'main'"
				]
			})
		}

		data "buildkite_organization_rule" "%s_rule" {
			depends_on = [buildkite_organization_rule.%s_rule]
			uuid = buildkite_organization_rule.%s_rule.uuid
		}

		`, fields[0], fields[1], fields[0], fields[1], fields[2], fields[2], fields[2], fields[2], fields[2], fields[2])
	}

	for _, action := range ruleActions {
		t.Run(fmt.Sprintf("loads a pipeline.%s.pipeline organization rule with required attributes by id", action), func(t *testing.T) {
			randdNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, fmt.Sprintf("buildkite_organization_rule.%s_rule", action)),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule data source's attributes are set in state
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "id"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "uuid"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "type"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequiredByID(randdNameOne, randNameTwo, action),
						Check:  check,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("loads a pipeline.%s.pipeline organization rule with all attributes by id", action), func(t *testing.T) {
			randdNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, fmt.Sprintf("buildkite_organization_rule.%s_rule", action)),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule data source's attributes are set in state
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "id"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "uuid"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "type"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "description"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllByID(randdNameOne, randNameTwo, action),
						Check:  check,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("loads a pipeline.%s.pipeline organization rule with required attributes by uuid", action), func(t *testing.T) {
			randdNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, fmt.Sprintf("buildkite_organization_rule.%s_rule", action)),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule data source's attributes are set in state
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "id"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "uuid"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "type"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configRequiredByUUID(randdNameOne, randNameTwo, action),
						Check:  check,
					},
				},
			})
		})

		t.Run(fmt.Sprintf("loads a pipeline.%s.pipeline organization rule with all attributes by uuid", action), func(t *testing.T) {
			randdNameOne := acctest.RandString(12)
			randNameTwo := acctest.RandString(12)
			var orr organizationRuleResourceModel

			check := resource.ComposeAggregateTestCheckFunc(
				// Confirm the organization rule exists
				testAccCheckOrganizationRuleExists(&orr, fmt.Sprintf("buildkite_organization_rule.%s_rule", action)),
				// Confirm the organization rule has the correct values in Buildkite's system
				testAccCheckOrganizationRuleRemoteValues(&orr, "PIPELINE", "PIPELINE", strings.ToUpper(action), "ALLOW"),
				// Check the organization rule data source's attributes are set in state
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "id"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "uuid"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "type"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "description"),
				resource.TestCheckResourceAttrSet(fmt.Sprintf("data.buildkite_organization_rule.%s_rule", action), "value"),
			)

			resource.ParallelTest(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckOrganizationRuleDestroy,
				Steps: []resource.TestStep{
					{
						Config: configAllByUUID(randdNameOne, randNameTwo, action),
						Check:  check,
					},
				},
			})
		})
	}
}
