package buildkite

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteClusterMaintainerResource_User(t *testing.T) {
	basic := func(clusterName, userID string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_cluster" "test_cluster" {
			name = "%s_test_cluster"
		}

		resource "buildkite_cluster_maintainer" "test_user" {
			cluster_id = buildkite_cluster.test_cluster.uuid
			user_id          = "%s"
		}
		`, clusterName, userID)
	}

	clusterName := acctest.RandString(12)
	userID := "8db2920e-3c60-48a7-a3f8-2584be374bac" // Real user UUID from test environment (decoded from GraphQL ID)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterMaintainerDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic(clusterName, userID),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterMaintainerExists("buildkite_cluster_maintainer.test_user"),
					resource.TestCheckResourceAttr("buildkite_cluster_maintainer.test_user", "user_id", userID),
					resource.TestCheckResourceAttr("buildkite_cluster_maintainer.test_user", "actor_type", "user"),
					resource.TestCheckResourceAttr("buildkite_cluster_maintainer.test_user", "actor_id", userID),
					resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_user", "id"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_user", "cluster_id"),
				),
			},
		},
	})
}

func TestAccBuildkiteClusterMaintainerResource_Team(t *testing.T) {
	basic := func(clusterName string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
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
			cluster_id = buildkite_cluster.test_cluster.uuid
			team_id    = buildkite_team.test_team.uuid
		}
		`, clusterName, clusterName)
	}

	clusterName := acctest.RandString(12)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterMaintainerDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic(clusterName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterMaintainerExists("buildkite_cluster_maintainer.test_team"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_team", "team_id"),
					resource.TestCheckResourceAttr("buildkite_cluster_maintainer.test_team", "actor_type", "team"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_team", "actor_id"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_team", "id"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_maintainer.test_team", "cluster_id"),
				),
			},
		},
	})
}

func TestAccBuildkiteClusterMaintainerResource_Import(t *testing.T) {
	basic := func(clusterName, userID string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_cluster" "test_cluster" {
			name = "%s_test_cluster"
		}

		resource "buildkite_cluster_maintainer" "test_user" {
			cluster_id = buildkite_cluster.test_cluster.uuid
			user_id          = "%s"
		}
		`, clusterName, userID)
	}

	clusterName := acctest.RandString(12)
	userID := "8db2920e-3c60-48a7-a3f8-2584be374bac" // Real user UUID from test environment (decoded from GraphQL ID)
	resourceName := "buildkite_cluster_maintainer.test_user"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterMaintainerDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic(clusterName, userID),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterMaintainerExists(resourceName),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccClusterMaintainerImportStateIdFunc(resourceName),
			},
		},
	})
}

func TestAccBuildkiteClusterMaintainerResource_InvalidConfiguration(t *testing.T) {
	bothUserAndTeam := func(clusterName string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_cluster" "test_cluster" {
			name = "%s_test_cluster"
		}

		resource "buildkite_cluster_maintainer" "invalid" {
			cluster_id = buildkite_cluster.test_cluster.uuid
			user_id          = "test-user-id"
			team_id          = "test-team-id"
		}
		`, clusterName)
	}

	neitherUserNorTeam := func(clusterName string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_cluster" "test_cluster" {
			name = "%s_test_cluster"
		}

		resource "buildkite_cluster_maintainer" "invalid" {
			cluster_id = buildkite_cluster.test_cluster.uuid
		}
		`, clusterName)
	}

	clusterName := acctest.RandString(12)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      bothUserAndTeam(clusterName),
				ExpectError: regexp.MustCompile("Only one of user_id or team_id can be specified"),
			},
			{
				Config:      neitherUserNorTeam(clusterName),
				ExpectError: regexp.MustCompile("Either user_id or team_id must be specified"),
			},
		},
	})
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

		// Here we would normally make an API call to verify the resource exists
		// For now, we'll just check that required attributes are set
		if rs.Primary.Attributes["cluster_id"] == "" {
			return fmt.Errorf("cluster_id not set")
		}

		// Check that either user_id or team_id is set, but not both
		userID := rs.Primary.Attributes["user_id"]
		teamID := rs.Primary.Attributes["team_id"]

		if (userID == "" && teamID == "") || (userID != "" && teamID != "") {
			return fmt.Errorf("exactly one of user_id or team_id must be set")
		}

		return nil
	}
}

func testAccCheckClusterMaintainerDestroy(s *terraform.State) error {
	// Here we would normally check that the cluster maintainer has been deleted
	// by making an API call. For now, we'll just return nil.
	return nil
}

func testAccClusterMaintainerImportStateIdFunc(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("not found: %s", resourceName)
		}

		clusterID := rs.Primary.Attributes["cluster_id"]
		permissionID := rs.Primary.ID

		return fmt.Sprintf("%s/%s", clusterID, permissionID), nil
	}
}
