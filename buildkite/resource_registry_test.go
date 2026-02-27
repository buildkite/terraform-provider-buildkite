package buildkite

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccResourceRegistry(t *testing.T) {
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

	configWithAllFields := func(name, ecosystem, description, emoji, color string) string {
		return fmt.Sprintf(`
		provider "buildkite" {}

		resource "buildkite_registry" "test" {
			name        = "%s"
			ecosystem   = "%s"
			description = "%s"
			emoji       = "%s"
			color       = "%s"
			team_ids    = ["31529c8a-7cfa-42e8-bb85-4c844a983ea0"]
		}`, name, ecosystem, description, emoji, color)
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
						resource.TestCheckResourceAttr("buildkite_registry.test", "public", "false"),
						resource.TestCheckResourceAttrSet("buildkite_registry.test", "registry_type"),
					),
				},
			},
		})
	})

	t.Run("create with all fields", func(t *testing.T) {
		randName := acctest.RandString(5)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: configWithAllFields(randName, "python", "Test registry description", ":snake:", "#3776AB"),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_registry.test", "name", randName),
						resource.TestCheckResourceAttr("buildkite_registry.test", "ecosystem", "python"),
						resource.TestCheckResourceAttr("buildkite_registry.test", "description", "Test registry description"),
						resource.TestCheckResourceAttr("buildkite_registry.test", "emoji", ":snake:"),
						resource.TestCheckResourceAttr("buildkite_registry.test", "color", "#3776AB"),
						resource.TestCheckResourceAttr("buildkite_registry.test", "public", "false"),
						resource.TestCheckResourceAttrSet("buildkite_registry.test", "registry_type"),
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

	t.Run("update description and color", func(t *testing.T) {
		randName := acctest.RandString(5)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: configWithAllFields(randName, "java", "Initial description", ":package:", "#FF0000"),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_registry.test", "description", "Initial description"),
						resource.TestCheckResourceAttr("buildkite_registry.test", "color", "#FF0000"),
					),
				},
				{
					Config: configWithAllFields(randName, "java", "Updated description", ":package:", "#00FF00"),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_registry.test", "description", "Updated description"),
						resource.TestCheckResourceAttr("buildkite_registry.test", "color", "#00FF00"),
					),
				},
			},
		})
	})

	t.Run("reject ecosystem change", func(t *testing.T) {
		randName := acctest.RandString(5)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(randName, "java", ":package:"),
				},
				{
					Config:      config(randName, "python", ":package:"),
					ExpectError: regexp.MustCompile(`Ecosystem change detected`),
				},
			},
		})
	})

	t.Run("reject team_ids change", func(t *testing.T) {
		randName := acctest.RandString(5)

		configWithTeams := func(name, teamID string) string {
			return fmt.Sprintf(`
			provider "buildkite" {}

			resource "buildkite_registry" "test" {
				name      = "%s"
				ecosystem = "java"
				team_ids  = ["%s"]
			}`, name, teamID)
		}

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: configWithTeams(randName, "31529c8a-7cfa-42e8-bb85-4c844a983ea0"),
				},
				{
					Config:      configWithTeams(randName, "00000000-0000-0000-0000-000000000000"),
					ExpectError: regexp.MustCompile(`Team IDs change detected`),
				},
			},
		})
	})

	t.Run("create with oidc policy", func(t *testing.T) {
		randName := acctest.RandString(5)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
					provider "buildkite" {}

					resource "buildkite_registry" "test" {
						name      = "%s"
						ecosystem = "java"
						oidc_policy = <<-YAML
						- iss: https://agent.buildkite.com
						  scopes:
						    - read_packages
						  claims:
						    organization_slug: %s
						YAML
						team_ids = ["31529c8a-7cfa-42e8-bb85-4c844a983ea0"]
					}`, randName, os.Getenv("BUILDKITE_ORGANIZATION_SLUG")),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("buildkite_registry.test", "name", randName),
						resource.TestCheckResourceAttrSet("buildkite_registry.test", "oidc_policy"),
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
