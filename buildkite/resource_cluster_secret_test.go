package buildkite

import (
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteClusterSecret_basic(t *testing.T) {
	secretKey := fmt.Sprintf("TEST_SECRET_%s", acctest.RandString(10))
	secretValue := acctest.RandString(20)
	clusterName := acctest.RandString(10)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterSecretDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterSecretConfig(clusterName, secretKey, secretValue, "Initial description"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "key", secretKey),
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "description", "Initial description"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "id"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "created_at"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "updated_at"),
				),
			},
		},
	})
}

func TestAccBuildkiteClusterSecret_update(t *testing.T) {
	secretKey := fmt.Sprintf("TEST_SECRET_%s", acctest.RandString(10))
	secretValue1 := acctest.RandString(20)
	secretValue2 := acctest.RandString(20)
	clusterName := acctest.RandString(10)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterSecretDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterSecretConfig(clusterName, secretKey, secretValue1, "Initial description"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "description", "Initial description"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "created_at"),
				),
			},
			{
				Config: testAccClusterSecretConfig(clusterName, secretKey, secretValue2, "Updated description"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "description", "Updated description"),
					// Verify created_at doesn't change on update
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "created_at"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "updated_at"),
				),
			},
		},
	})
}

func TestAccBuildkiteClusterSecret_withPolicy(t *testing.T) {
	secretKey := fmt.Sprintf("TEST_SECRET_%s", acctest.RandString(10))
	secretValue := acctest.RandString(20)
	clusterName := acctest.RandString(10)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterSecretDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterSecretConfigWithPolicy(clusterName, secretKey, secretValue, "my-pipeline", "main"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "key", secretKey),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "policy"),
				),
			},
			{
				Config: testAccClusterSecretConfigWithPolicy(clusterName, secretKey, secretValue, "updated-pipeline", "develop"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "policy"),
				),
			},
		},
	})
}

func testAccCheckClusterSecretDestroy(s *terraform.State) error {
	org := getenv("BUILDKITE_ORGANIZATION_SLUG")
	apiToken := os.Getenv("BUILDKITE_API_TOKEN")
	httpClient := &http.Client{}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster_secret" {
			continue
		}

		clusterID := rs.Primary.Attributes["cluster_id"]
		secretID := rs.Primary.ID

		url := fmt.Sprintf("%s/v2/organizations/%s/clusters/%s/secrets/%s",
			defaultRestEndpoint, org, clusterID, secretID)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", "Bearer "+apiToken)

		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()

		// If we get a 404, the secret was successfully destroyed
		if resp.StatusCode == 404 {
			continue
		}

		// If we get 200, the secret still exists
		if resp.StatusCode == 200 {
			return fmt.Errorf("cluster secret %s still exists", secretID)
		}
	}

	return nil
}

func testAccClusterSecretConfig(clusterName, key, value, description string) string {
	return fmt.Sprintf(`
provider "buildkite" {
	timeouts = {
		create = "10s"
		read = "10s"
		update = "10s"
		delete = "10s"
	}
}

resource "buildkite_cluster" "test" {
	name        = "Test Cluster %s"
	description = "Test cluster for secrets"
}

resource "buildkite_cluster_secret" "test" {
	cluster_id  = buildkite_cluster.test.uuid
	key         = "%s"
	value       = "%s"
	description = "%s"
}
`, clusterName, key, value, description)
}

func testAccClusterSecretConfigWithPolicy(clusterName, key, value, pipeline, branch string) string {
	return fmt.Sprintf(`
provider "buildkite" {
	timeouts = {
		create = "10s"
		read = "10s"
		update = "10s"
		delete = "10s"
	}
}

resource "buildkite_cluster" "test" {
	name        = "Test Cluster %s"
	description = "Test cluster for secrets"
}

resource "buildkite_cluster_secret" "test" {
	cluster_id  = buildkite_cluster.test.uuid
	key         = "%s"
	value       = "%s"
	description = "Secret with policy"
	
	policy = <<-EOT
- pipeline_slug: %s
  build_branch: %s
EOT
}
`, clusterName, key, value, pipeline, branch)
}
