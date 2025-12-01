package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteClusterMaintainerResource(t *testing.T) {
	t.Run("adds a user as maintainer", func(t *testing.T) {
		config := func(clusterName, userID string) string {
			return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "test_cluster" {
				name = "%s_test_cluster"
			}

			resource "buildkite_cluster_maintainer" "test_user" {
				cluster_uuid = buildkite_cluster.test_cluster.uuid
				user_uuid    = "%s"
			}
			`, clusterName, userID)
		}

		clusterName := acctest.RandString(12)
		userID := "8db2920e-3c60-48a7-a3f8-2584be374bac" // Real user UUID from test environment (decoded from GraphQL ID)

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterMaintainerExists("buildkite_cluster_maintainer.test_user"),
			resource.TestCheckResourceAttr("buildkite_cluster_maintainer.test_user", "user_uuid", userID),
			resource.TestCheckResourceAttr("buildkite_cluster_maintainer.test_user", "actor_type", "user"),
			resource.TestCheckResourceAttr("buildkite_cluster_maintainer.test_user", "actor_uuid", userID),
			resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_user", "id"),
			resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_user", "cluster_uuid"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterMaintainerDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(clusterName, userID),
					Check:  check,
				},
			},
		})
	})

	t.Run("adds a team as maintainer", func(t *testing.T) {
		config := func(clusterName string) string {
			return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "test_cluster" {
				name = "%s_test_cluster"
			}

			resource "buildkite_team" "test_team" {
				name = "%s_test_team"
				description = "Test team for cluster maintainer tests"
				privacy = "VISIBLE"
				default_team = false
				default_member_role = "MEMBER"
				members_can_create_pipelines = false
			}

			resource "buildkite_cluster_maintainer" "test_team" {
				cluster_uuid = buildkite_cluster.test_cluster.uuid
				team_uuid    = buildkite_team.test_team.uuid
			}
			`, clusterName, clusterName)
		}

		clusterName := acctest.RandString(12)

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterMaintainerExists("buildkite_cluster_maintainer.test_team"),
			resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_team", "team_uuid"),
			resource.TestCheckResourceAttr("buildkite_cluster_maintainer.test_team", "actor_type", "team"),
			resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_team", "actor_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_team", "id"),
			resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_team", "cluster_uuid"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterMaintainerDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(clusterName),
					Check:  check,
				},
			},
		})
	})

	t.Run("imports a cluster maintainer", func(t *testing.T) {
		config := func(clusterName, userID string) string {
			return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "test_cluster" {
				name = "%s_test_cluster"
			}

			resource "buildkite_cluster_maintainer" "test_user" {
				cluster_uuid = buildkite_cluster.test_cluster.uuid
				user_uuid    = "%s"
			}
			`, clusterName, userID)
		}

		clusterName := acctest.RandString(12)
		userID := "8db2920e-3c60-48a7-a3f8-2584be374bac" // Real user UUID from test environment (decoded from GraphQL ID)
		resourceName := "buildkite_cluster_maintainer.test_user"

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterMaintainerExists(resourceName),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterMaintainerDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(clusterName, userID),
					Check:  check,
				},
				{
					ResourceName:      resourceName,
					ImportState:       true,
					ImportStateVerify: true,
					ImportStateIdFunc: testAccClusterMaintainerImportStateIdFunc(resourceName),
				},
			},
		})
	})

	t.Run("validates configuration", func(t *testing.T) {
		configBothUserAndTeam := func(clusterName string) string {
			return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "test_cluster" {
				name = "%s_test_cluster"
			}

			resource "buildkite_cluster_maintainer" "invalid" {
				cluster_uuid = buildkite_cluster.test_cluster.uuid
				user_uuid    = "test-user-id"
				team_uuid    = "test-team-id"
			}
			`, clusterName)
		}

		configNeitherUserNorTeam := func(clusterName string) string {
			return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "test_cluster" {
				name = "%s_test_cluster"
			}

			resource "buildkite_cluster_maintainer" "invalid" {
				cluster_uuid = buildkite_cluster.test_cluster.uuid
			}
			`, clusterName)
		}

		clusterName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:      configBothUserAndTeam(clusterName),
					ExpectError: regexp.MustCompile("Only one of user_uuid or team_uuid can be specified"),
				},
				{
					Config:      configNeitherUserNorTeam(clusterName),
					ExpectError: regexp.MustCompile("Either user_uuid or team_uuid must be specified"),
				},
			},
		})
	})

	t.Run("upgrades state from v0 to v1", func(t *testing.T) {
		// Test that state upgrade from v0 to v1 works correctly
		// This ensures existing resources with cluster_id, user_id, team_id, and actor_id
		// are automatically migrated to cluster_uuid, user_uuid, team_uuid, and actor_uuid

		clusterName := acctest.RandString(12)
		userID := "8db2920e-3c60-48a7-a3f8-2584be374bac" // Real user UUID from test environment

		// Config for old provider version (uses old attribute names)
		configV0 := fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "test_cluster" {
				name = "%s_test_cluster"
			}

			resource "buildkite_cluster_maintainer" "test_user" {
				cluster_id = buildkite_cluster.test_cluster.uuid
				user_id    = "%s"
			}
		`, clusterName, userID)

		// Config for new provider version (uses new attribute names)
		configV1 := fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "test_cluster" {
				name = "%s_test_cluster"
			}

			resource "buildkite_cluster_maintainer" "test_user" {
				cluster_uuid = buildkite_cluster.test_cluster.uuid
				user_uuid    = "%s"
			}
		`, clusterName, userID)

		// Check for v0 - validates old attribute names
		checkV0 := resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("buildkite_cluster_maintainer.test_user", "user_id", userID),
			resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_user", "cluster_id"),
			resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_user", "actor_id"),
			resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_user", "id"),
		)

		// Check for v1 - validates new attribute names after migration
		checkV1 := resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("buildkite_cluster_maintainer.test_user", "user_uuid", userID),
			resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_user", "cluster_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_user", "actor_uuid"),
			resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_user", "id"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck: func() { testAccPreCheck(t) },
			Steps: []resource.TestStep{
				{
					ExternalProviders: map[string]resource.ExternalProvider{
						"buildkite": {
							VersionConstraint: "1.26.0", // Version before the rename
							Source:            "buildkite/buildkite",
						},
					},
					Config: configV0,
					Check:  checkV0,
				},
				{
					ProtoV6ProviderFactories: protoV6ProviderFactories(),
					Config:                   configV1,
					Check:                    checkV1,
				},
			},
		})
	})
}

func getTestClient() *Client {
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+os.Getenv("BUILDKITE_API_TOKEN"))
	header.Set("User-Agent", "terraform-provider-buildkite-test")

	httpClient := &http.Client{
		Transport: newHeaderRoundTripper(http.DefaultTransport, header),
	}

	client := &Client{
		http:         httpClient,
		organization: getenv("BUILDKITE_ORGANIZATION_SLUG"),
		restURL:      defaultRestEndpoint,
	}

	return client
}

func testAccCheckClusterMaintainerExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no cluster maintainer ID is set")
		}

		clusterUUID := rs.Primary.Attributes["cluster_uuid"]
		if clusterUUID == "" {
			return fmt.Errorf("cluster_uuid not set")
		}

		// Check that either user_uuid or team_uuid is set, but not both
		userUUID := rs.Primary.Attributes["user_uuid"]
		teamUUID := rs.Primary.Attributes["team_uuid"]

		if (userUUID == "" && teamUUID == "") || (userUUID != "" && teamUUID != "") {
			return fmt.Errorf("exactly one of user_uuid or team_uuid must be set")
		}

		// Make an API call to verify the cluster maintainer exists
		client := getTestClient()
		path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/maintainers/%s",
			client.organization,
			clusterUUID,
			rs.Primary.ID,
		)

		var result clusterMaintainerAPIResponse
		err := client.makeRequest(context.Background(), http.MethodGet, path, nil, &result)
		if err != nil {
			if strings.Contains(err.Error(), "404") {
				return fmt.Errorf("cluster maintainer %s not found in API", rs.Primary.ID)
			}
			return fmt.Errorf("error fetching cluster maintainer from API: %v", err)
		}

		// Verify the maintainer details match
		if result.ID != rs.Primary.ID {
			return fmt.Errorf("cluster maintainer ID mismatch: API returned %s, expected %s", result.ID, rs.Primary.ID)
		}

		return nil
	}
}

func testAccCheckClusterMaintainerDestroy(s *terraform.State) error {
	client := getTestClient()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster_maintainer" {
			continue
		}

		clusterUUID := rs.Primary.Attributes["cluster_uuid"]
		path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/maintainers/%s",
			client.organization,
			clusterUUID,
			rs.Primary.ID,
		)

		var result clusterMaintainerAPIResponse
		err := client.makeRequest(context.Background(), http.MethodGet, path, nil, &result)
		if err != nil {
			// If we get a 404, the maintainer was successfully deleted
			if strings.Contains(err.Error(), "404") {
				continue
			}
			return fmt.Errorf("error checking if cluster maintainer still exists: %v", err)
		}

		// If we get here, the maintainer still exists, which means destroy failed
		return fmt.Errorf("cluster maintainer %s still exists", rs.Primary.ID)
	}

	return nil
}

func testAccClusterMaintainerImportStateIdFunc(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("not found: %s", resourceName)
		}

		clusterUUID := rs.Primary.Attributes["cluster_uuid"]
		permissionID := rs.Primary.ID

		return fmt.Sprintf("%s/%s", clusterUUID, permissionID), nil
	}
}
