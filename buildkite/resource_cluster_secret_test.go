package buildkite

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteClusterSecretResource(t *testing.T) {
	configBasic := func(clusterName, secretKey, secretValue, description string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_test" {
			name = "Test cluster %s"
			description = "Acceptance testing cluster"
		}

		resource "buildkite_cluster_secret" "test_secret" {
			cluster_id = buildkite_cluster.cluster_test.id
			key = "%s"
			value = "%s"
			description = "%s"
		}
		`, clusterName, secretKey, secretValue, description)
	}

	configWithPolicy := func(clusterName, secretKey, secretValue, description, policy string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_test" {
			name = "Test cluster %s"
			description = "Acceptance testing cluster"
		}

		resource "buildkite_cluster_secret" "test_secret" {
			cluster_id = buildkite_cluster.cluster_test.id
			key = "%s"
			value = "%s"
			description = "%s"
			policy = <<-EOT
%s
			EOT
		}
		`, clusterName, secretKey, secretValue, description, policy)
	}

	t.Run("creates a cluster secret", func(t *testing.T) {
		clusterName := acctest.RandString(10)
		secretKey := "TEST_SECRET_" + acctest.RandString(5)
		secretValue := acctest.RandString(20)
		description := "Test secret description"

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterSecretExists("buildkite_cluster_secret.test_secret"),
			resource.TestCheckResourceAttr("buildkite_cluster_secret.test_secret", "key", secretKey),
			resource.TestCheckResourceAttr("buildkite_cluster_secret.test_secret", "description", description),
			resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test_secret", "id"),
			resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test_secret", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test_secret", "cluster_uuid"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterSecretDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(clusterName, secretKey, secretValue, description),
					Check:  check,
				},
			},
		})
	})

	t.Run("creates a cluster secret with policy", func(t *testing.T) {
		clusterName := acctest.RandString(10)
		secretKey := "TEST_SECRET_" + acctest.RandString(5)
		secretValue := acctest.RandString(20)
		description := "Test secret with policy"
		policy := "- pipeline_slug: my-pipeline\n  build_branch: main"

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterSecretExists("buildkite_cluster_secret.test_secret"),
			resource.TestCheckResourceAttr("buildkite_cluster_secret.test_secret", "key", secretKey),
			resource.TestCheckResourceAttr("buildkite_cluster_secret.test_secret", "description", description),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterSecretDestroy,
			Steps: []resource.TestStep{
				{
					Config: configWithPolicy(clusterName, secretKey, secretValue, description, policy),
					Check:  check,
				},
			},
		})
	})

	t.Run("updates a cluster secret description", func(t *testing.T) {
		clusterName := acctest.RandString(10)
		secretKey := "TEST_SECRET_" + acctest.RandString(5)
		secretValue := acctest.RandString(20)
		description := "Original description"
		updatedDescription := "Updated description"

		checkInitial := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterSecretExists("buildkite_cluster_secret.test_secret"),
			resource.TestCheckResourceAttr("buildkite_cluster_secret.test_secret", "description", description),
		)

		checkUpdated := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterSecretExists("buildkite_cluster_secret.test_secret"),
			resource.TestCheckResourceAttr("buildkite_cluster_secret.test_secret", "description", updatedDescription),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterSecretDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(clusterName, secretKey, secretValue, description),
					Check:  checkInitial,
				},
				{
					Config: configBasic(clusterName, secretKey, secretValue, updatedDescription),
					Check:  checkUpdated,
				},
			},
		})
	})

	t.Run("updates a cluster secret value", func(t *testing.T) {
		clusterName := acctest.RandString(10)
		secretKey := "TEST_SECRET_" + acctest.RandString(5)
		secretValue := acctest.RandString(20)
		newSecretValue := acctest.RandString(20)
		description := "Test secret"

		checkInitial := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterSecretExists("buildkite_cluster_secret.test_secret"),
			resource.TestCheckResourceAttr("buildkite_cluster_secret.test_secret", "value", secretValue),
		)

		checkUpdated := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterSecretExists("buildkite_cluster_secret.test_secret"),
			resource.TestCheckResourceAttr("buildkite_cluster_secret.test_secret", "value", newSecretValue),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterSecretDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(clusterName, secretKey, secretValue, description),
					Check:  checkInitial,
				},
				{
					Config: configBasic(clusterName, secretKey, newSecretValue, description),
					Check:  checkUpdated,
				},
			},
		})
	})

	t.Run("replacing key requires replace", func(t *testing.T) {
		clusterName := acctest.RandString(10)
		secretKey := "TEST_SECRET_" + acctest.RandString(5)
		newSecretKey := "TEST_SECRET_" + acctest.RandString(5)
		secretValue := acctest.RandString(20)
		description := "Test secret"

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterSecretDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(clusterName, secretKey, secretValue, description),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_cluster_secret.test_secret", "key", secretKey),
					),
				},
				{
					Config: configBasic(clusterName, newSecretKey, secretValue, description),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_cluster_secret.test_secret", "key", newSecretKey),
					),
				},
			},
		})
	})
}

func testAccCheckClusterSecretExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		r, err := getNode(
			context.Background(),
			genqlientGraphql,
			resourceState.Primary.ID,
		)
		if err != nil {
			return fmt.Errorf("Error fetching Cluster Secret from graphql API: %v", err)
		}

		if _, ok := r.Node.(*getNodeNodeSecret); !ok {
			return fmt.Errorf("No Cluster Secret found with graphql id: %s", resourceState.Primary.ID)
		}

		return nil
	}
}

func testAccCheckClusterSecretDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster_secret" {
			continue
		}

		r, err := getNode(
			context.Background(),
			genqlientGraphql,
			rs.Primary.ID,
		)
		if err != nil {
			return fmt.Errorf("Error fetching Cluster Secret from graphql API: %v", err)
		}

		if secret, ok := r.Node.(*getNodeNodeSecret); ok {
			if secret.Id != "" {
				return fmt.Errorf("Cluster Secret still exists: %s", rs.Primary.ID)
			}
		}
	}
	return nil
}
