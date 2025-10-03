package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkitePortal(t *testing.T) {
	basic := func(name string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "10s"
				read = "10s"
				update = "10s"
				delete = "10s"
			}
		}

		resource "buildkite_portal" "test" {
			slug = %q
			name = "Test Portal %s"
			description = "Test portal description"
			query = "{ viewer { user { name } } }"
			user_invokable = false
		}
		`, fmt.Sprintf("test-portal-%s", name), name)
	}

	t.Run("creates a portal", func(t *testing.T) {
		randName := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckPortalExists("buildkite_portal.test"),
			resource.TestCheckResourceAttr("buildkite_portal.test", "name", fmt.Sprintf("Test Portal %s", randName)),
			resource.TestCheckResourceAttr("buildkite_portal.test", "description", "Test portal description"),
			resource.TestCheckResourceAttr("buildkite_portal.test", "user_invokable", "false"),
			resource.TestCheckResourceAttrSet("buildkite_portal.test", "uuid"),
			resource.TestCheckResourceAttrSet("buildkite_portal.test", "token"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPortalResourceDestroy,
			Steps: []resource.TestStep{
				{
					Config: basic(randName),
					Check:  check,
				},
				{
					ResourceName: "buildkite_portal.test",
					ImportState:  true,
					ImportStateIdFunc: func(s *terraform.State) (string, error) {
						rs, ok := s.RootModule().Resources["buildkite_portal.test"]
						if !ok {
							return "", fmt.Errorf("resource not found: %s", "buildkite_portal.test")
						}
						return rs.Primary.Attributes["slug"], nil
					},
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("updates a portal", func(t *testing.T) {
		randName := acctest.RandString(10)
		updated := func(name string) string {
			return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "10s"
					read = "10s"
					update = "10s"
					delete = "10s"
				}
			}

			resource "buildkite_portal" "test" {
				slug = %q
				name = "Updated Portal %s"
				description = "Updated description"
				query = "{ viewer { user { email } } }"
				user_invokable = true
			}
			`, fmt.Sprintf("test-portal-%s", name), name)
		}

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckPortalExists("buildkite_portal.test"),
			resource.TestCheckResourceAttr("buildkite_portal.test", "name", fmt.Sprintf("Test Portal %s", randName)),
		)

		checkUpdated := resource.ComposeAggregateTestCheckFunc(
			testAccCheckPortalExists("buildkite_portal.test"),
			resource.TestCheckResourceAttr("buildkite_portal.test", "name", fmt.Sprintf("Updated Portal %s", randName)),
			resource.TestCheckResourceAttr("buildkite_portal.test", "description", "Updated description"),
			resource.TestCheckResourceAttr("buildkite_portal.test", "user_invokable", "true"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPortalResourceDestroy,
			Steps: []resource.TestStep{
				{
					Config: basic(randName),
					Check:  check,
				},
				{
					Config: updated(randName),
					Check:  checkUpdated,
				},
			},
		})
	})
}

func testAccCheckPortalExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("not found in state: %s", resourceName)
		}

		if resourceState.Primary.Attributes["slug"] == "" {
			return fmt.Errorf("no slug is set in state")
		}

		client := getTestClient()
		path := fmt.Sprintf("/v2/organizations/%s/portals/%s",
			client.organization,
			resourceState.Primary.Attributes["slug"],
		)

		var result portalAPIResponse
		err := client.makeRequest(context.Background(), http.MethodGet, path, nil, &result)
		if err != nil {
			return fmt.Errorf("error fetching portal from API: %v", err)
		}

		if result.UUID == "" {
			return fmt.Errorf("no portal found with slug: %s", resourceState.Primary.Attributes["slug"])
		}

		return nil
	}
}

func testAccCheckPortalResourceDestroy(s *terraform.State) error {
	client := getTestClient()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_portal" {
			continue
		}

		path := fmt.Sprintf("/v2/organizations/%s/portals/%s",
			client.organization,
			rs.Primary.Attributes["slug"],
		)

		var result portalAPIResponse
		err := client.makeRequest(context.Background(), http.MethodGet, path, nil, &result)
		if err == nil {
			return fmt.Errorf("portal still exists")
		}
	}
	return nil
}
