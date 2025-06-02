package buildkite

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccResourceRegistry(t *testing.T) {
	RegisterResourceTracking(t)
	config := func(name, ecosystem, emoji string) string {
		return fmt.Sprintf(`
		provider "buildkite" {}

		resource "buildkite_registry" "test" {
			name = "%s"
			ecosystem = "%s"
			emoji = "%s"
			team_ids = ["31529c8a-7cfa-42e8-bb85-4c844a983ea0"]
		}`, name, ecosystem, emoji)
	}

	t.Run("create and destroy", func(t *testing.T) {
		randName := acctest.RandString(5)
		ecosystem := "java"

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(randName, ecosystem, ":buildkite:"),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_registry.test", "name", randName),
						resource.TestCheckResourceAttr("buildkite_registry.test", "ecosystem", ecosystem),
						resource.TestCheckResourceAttrSet("buildkite_registry.test", "id"),
						resource.TestCheckResourceAttrSet("buildkite_registry.test", "uuid"),
						resource.TestCheckResourceAttrSet("buildkite_registry.test", "slug"),
					),
				},
			},
		})
	})

	t.Run("update", func(t *testing.T) {
		randName := acctest.RandString(5)
		ecosystem := "java"

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(randName, ecosystem, ":bazel:"),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_registry.test", "name", randName),
						resource.TestCheckResourceAttr("buildkite_registry.test", "emoji", ":bazel:"),
					),
				},
				{
					Config: config(randName, ecosystem, ":buildkite:"),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_registry.test", "name", randName),
						resource.TestCheckResourceAttr("buildkite_registry.test", "emoji", ":buildkite:"),
					),
				},
			},
		})
	})

	t.Run("import", func(t *testing.T) {
		var r registryResourceModel
		randName := acctest.RandString(5)
		ecosystem := "java"

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(randName, ecosystem, ":bazel:"),
					Check: resource.ComposeAggregateTestCheckFunc(
						testAccCheckRegistryExists("buildkite_registry.test", &r),
						resource.TestCheckResourceAttr("buildkite_registry.test", "name", randName),
					),
				},
				{
					ResourceName: "buildkite_registry.test",
					ImportState:  true,
					ImportStateIdFunc: func(s *terraform.State) (string, error) {
						// Get the slug from the state to use for import
						rs, ok := s.RootModule().Resources["buildkite_registry.test"]
						if !ok {
							return "", fmt.Errorf("resource not found: %s", "buildkite_registry.test")
						}
						// Use the slug as the import ID
						return rs.Primary.Attributes["slug"], nil
					},
					ImportStateVerify:       true,
					ImportStateVerifyIgnore: []string{"team_ids"},
				},
			},
		})
	})
}

func testAccCheckRegistryDestroy(s *terraform.State) error {
	// Get client config from environment (similar to how PreCheck would work)
	apiToken := os.Getenv("BUILDKITE_API_TOKEN")
	orgSlug := os.Getenv("BUILDKITE_ORGANIZATION_SLUG")
	baseURL := os.Getenv("BUILDKITE_API_URL")
	if baseURL == "" {
		baseURL = "https://api.buildkite.com"
	}

	// Create a simple HTTP client for testing
	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}

	// Find any resources of type buildkite_registry
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_registry" {
			continue
		}

		// Get the registry slug from the state
		slug := rs.Primary.Attributes["slug"]

		// Make API call to check if the registry still exists
		url := fmt.Sprintf("%s/v2/packages/organizations/%s/registries/%s", baseURL, orgSlug, slug)

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("error creating request to check if registry still exists: %w", err)
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))

		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("error making request to check if registry still exists: %w", err)
		}
		defer resp.Body.Close()

		// If we get a 404, the resource was successfully deleted
		if resp.StatusCode == http.StatusNotFound {
			continue
		}

		// If we get here, the resource still exists
		return fmt.Errorf("buildkite_registry resource still exists: %s", slug)
	}

	return nil
}

func testAccCheckRegistryExists(n string, r *registryResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		// Ensure the slug is set, which is critical for import
		if rs.Primary.Attributes["slug"] == "" {
			return fmt.Errorf("slug attribute is not set")
		}

		return nil
	}
}
