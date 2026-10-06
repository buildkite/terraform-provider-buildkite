package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteClusterResource(t *testing.T) {
	basic := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "foo" {
			name = "%s_test_cluster"
		}
		`, name)
	}

	complex := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "foo" {
			name = "%s_test_cluster"
			description = "Just another Buildkite cluster"
			emoji = "%s"
			color = "%s"
		}
		`, fields[0], fields[1], fields[2])
	}

	t.Run("Creates a Cluster with basic settings", func(t *testing.T) {
		var c clusterResourceModel
		randName := acctest.RandString(5)
		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterExists("buildkite_cluster.foo", &c),
			testAccCheckClusterRemoteValues(&c, fmt.Sprintf("%s_test_cluster", randName)),
			resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randName)),
			resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "id"),
			resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "uuid"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterDestroy,
			Steps: []resource.TestStep{
				{
					Config: basic(randName),
					Check:  check,
				},
			},
		})
	})

	t.Run("Creates a Cluster with complex settings", func(t *testing.T) {
		var c clusterResourceModel
		randName := acctest.RandString(5)
		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterExists("buildkite_cluster.foo", &c),
			testAccCheckClusterRemoteValues(&c, fmt.Sprintf("%s_test_cluster", randName)),
			resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randName)),
			resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "id"),
			resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "uuid"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterDestroy,
			Steps: []resource.TestStep{
				{
					Config: complex(randName, ":triple-green-shell:", "#6932f9"),
					Check:  check,
				},
			},
		})
	})

	t.Run("Updates a Cluster using complex settings", func(t *testing.T) {
		var c clusterResourceModel
		randName := acctest.RandString(5)
		randNameUpdated := acctest.RandString(5)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterDestroy,
			Steps: []resource.TestStep{
				{
					Config: complex(randName, ":one-does-not-simply:", "#BADA55"),
					Check: resource.ComposeAggregateTestCheckFunc(
						testAccCheckClusterExists("buildkite_cluster.foo", &c),
						testAccCheckClusterRemoteValues(&c, fmt.Sprintf("%s_test_cluster", randName)),
						resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randName)),
						resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "id"),
						resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "uuid"),
						resource.TestCheckResourceAttr("buildkite_cluster.foo", "emoji", ":one-does-not-simply:"),
						resource.TestCheckResourceAttr("buildkite_cluster.foo", "color", "#BADA55"),
					),
				},
				{
					Config: complex(randNameUpdated, ":terraform:", "#b31625"),
					Check: resource.ComposeAggregateTestCheckFunc(
						testAccCheckClusterExists("buildkite_cluster.foo", &c),
						testAccCheckClusterRemoteValues(&c, fmt.Sprintf("%s_test_cluster", randNameUpdated)),
						resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randNameUpdated)),
						resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "id"),
						resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "uuid"),
						resource.TestCheckResourceAttrSet("buildkite_cluster.foo", "description"),
						resource.TestCheckResourceAttr("buildkite_cluster.foo", "emoji", ":terraform:"),
						resource.TestCheckResourceAttr("buildkite_cluster.foo", "color", "#b31625"),
					),
				},
			},
		})
	})

	t.Run("Imports a Cluster", func(t *testing.T) {
		var c clusterResourceModel
		randName := acctest.RandString(5)
		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterExists("buildkite_cluster.foo", &c),
			resource.TestCheckResourceAttr("buildkite_cluster.foo", "name", fmt.Sprintf("%s_test_cluster", randName)),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterDestroy,
			Steps: []resource.TestStep{
				{
					Config: basic(randName),
					Check:  check,
				},
				{
					ResourceName:      "buildkite_cluster.foo",
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})
}

// TestAccBuildkiteClusterAgentTracingService needs an organization with agent tracing enabled. It makes
// its own eligible service: an enabled OpenTelemetry tracing service covering all pipelines and branches.
func TestAccBuildkiteClusterAgentTracingService(t *testing.T) {
	randName := acctest.RandString(10)
	config := func(attribute string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_notification_service" "otel" {
			provider_type = "open_telemetry_tracing"
			description   = "Terraform acceptance test %[1]s"

			open_telemetry_tracing = {
				endpoint = "https://otel.example.com/%[1]s"
			}
		}

		resource "buildkite_cluster" "foo" {
			name = "%[1]s_test_cluster"
			%[2]s
		}
		`, randName, attribute)
	}

	const name = "buildkite_cluster.foo"
	selected := "agent_tracing_service_uuid = buildkite_notification_service.otel.id"
	cleared := `agent_tracing_service_uuid = ""`

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy: func(s *terraform.State) error {
			if err := testAccCheckClusterDestroy(s); err != nil {
				return err
			}
			return testAccCheckNotificationServiceDestroy(s)
		},
		Steps: []resource.TestStep{
			{
				Config: config(selected),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(name, "agent_tracing_service_uuid", "buildkite_notification_service.otel", "id"),
					testAccCheckClusterAgentTracingService(name, "buildkite_notification_service.otel"),
				),
			},
			{
				ResourceName:      name,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// removing the attribute keeps the selection
				Config: config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(name, "agent_tracing_service_uuid", "buildkite_notification_service.otel", "id"),
					testAccCheckClusterAgentTracingService(name, "buildkite_notification_service.otel"),
				),
			},
			{
				Config: config(cleared),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "agent_tracing_service_uuid", ""),
					testAccCheckClusterAgentTracingService(name, ""),
				),
			},
			{
				// removing the attribute after clearing plans nothing
				Config: config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: testAccCheckClusterAgentTracingService(name, ""),
			},
		},
	})
}

// testAccCheckClusterAgentTracingService checks the selection the API reports for a cluster: the id of the
// service resource named, or none when service is ""
func testAccCheckClusterAgentTracingService(cluster, service string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		want := ""
		if service != "" {
			rs, ok := s.RootModule().Resources[service]
			if !ok {
				return fmt.Errorf("Not found in state: %s", service)
			}
			want = rs.Primary.Attributes["id"]
		}

		rs, ok := s.RootModule().Resources[cluster]
		if !ok {
			return fmt.Errorf("Not found in state: %s", cluster)
		}
		r, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)
		if err != nil {
			return err
		}
		clusterNode, ok := r.GetNode().(*getNodeNodeCluster)
		if !ok || clusterNode == nil {
			return fmt.Errorf("Cluster not found: %s", rs.Primary.ID)
		}

		got := ""
		if clusterNode.AgentTracingServiceUuid != nil {
			got = *clusterNode.AgentTracingServiceUuid
		}
		if got != want {
			return fmt.Errorf("the API reports agent tracing service %q, want %q", got, want)
		}
		return nil
	}
}

func testAccCheckClusterExists(name string, c *clusterResourceModel) resource.TestCheckFunc {
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

		if clusterNode, ok := r.GetNode().(*getNodeNodeCluster); ok {
			if clusterNode == nil {
				return fmt.Errorf("Cluster not found: nil response")
			}
			updateClusterResourceState(c, *clusterNode)
		}
		return nil
	}
}

func testAccCheckClusterRemoteValues(c *clusterResourceModel, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if c.Name.ValueString() != name {
			return fmt.Errorf("unexpected name: %s, wanted: %s", c.Name, name)
		}
		return nil
	}
}

func testAccCheckClusterDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster" {
			continue
		}
	}
	return nil
}
