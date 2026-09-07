package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/shurcooL/graphql"
)

func TestHandleTeamIDs(t *testing.T) {
	t.Parallel()

	list := func(ids ...string) types.List {
		values := make([]attr.Value, len(ids))
		for i, id := range ids {
			values[i] = types.StringValue(id)
		}
		return types.ListValueMust(types.StringType, values)
	}
	null := types.ListNull(types.StringType)

	testCases := []struct {
		name     string
		api      []string
		existing types.List
		want     types.List
	}{
		{"configured teams are kept when another team is granted access", []string{"a", "b"}, list("a"), list("a")},
		{"configured teams are kept in the configured order", []string{"b", "a"}, list("a", "b"), list("a", "b")},
		{"api teams are used after import", []string{"a", "b"}, null, list("a", "b")},
		{"no teams anywhere is null", nil, null, null},
		{"no teams anywhere is null for an empty list", nil, list(), null},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := handleTeamIDs(tc.api, tc.existing); !got.Equal(tc.want) {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

const testRegistryTeamID = "31529c8a-7cfa-42e8-bb85-4c844a983ea0"

func TestAccResourceRegistry(t *testing.T) {
	config := func(name, ecosystem, emoji string) string {
		return fmt.Sprintf(`
		provider "buildkite" {}

		resource "buildkite_registry" "test" {
			name = "%s"
			ecosystem = "%s"
			emoji = "%s"
			team_ids = ["%s"]
		}`, name, ecosystem, emoji, testRegistryTeamID)
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
			team_ids    = ["%s"]
		}`, name, ecosystem, description, emoji, color, testRegistryTeamID)
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
					Config: configWithTeams(randName, testRegistryTeamID),
				},
				{
					Config:      configWithTeams(randName, "00000000-0000-0000-0000-000000000000"),
					ExpectError: regexp.MustCompile(`Team IDs change detected`),
				},
			},
		})
	})

	t.Run("keeps team_ids when another team is granted access", func(t *testing.T) {
		randName := acctest.RandString(5)

		config := fmt.Sprintf(`
			provider "buildkite" {}

			resource "buildkite_team" "owner" {
				name = "registry owner %s"
				privacy = "VISIBLE"
				default_team = false
				default_member_role = "MEMBER"
			}

			resource "buildkite_team" "other" {
				name = "registry other %s"
				privacy = "VISIBLE"
				default_team = false
				default_member_role = "MEMBER"
			}

			resource "buildkite_registry" "test" {
				name      = "%s"
				ecosystem = "java"
				team_ids  = [buildkite_team.owner.uuid]
			}`, randName, randName, randName)

		// Grant the other team access outside of team_ids, as the UI would
		grantOtherTeam := func(s *terraform.State) error {
			registry := s.RootModule().Resources["buildkite_registry.test"]
			team := s.RootModule().Resources["buildkite_team.other"]
			var mutation struct {
				TeamRegistryCreate struct {
					TeamRegistry struct {
						ID string `graphql:"id"`
					} `graphql:"teamRegistry"`
				} `graphql:"teamRegistryCreate(input: {teamID: $teamId, registryID: $registryId, accessLevel: READ_ONLY})"`
			}
			return graphqlClient.Mutate(context.Background(), &mutation, map[string]interface{}{
				"teamId":     graphql.ID(team.Primary.Attributes["id"]),
				"registryId": graphql.ID(registry.Primary.Attributes["id"]),
			})
		}

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: config,
					Check: resource.ComposeAggregateTestCheckFunc(
						grantOtherTeam,
						resource.TestCheckResourceAttr("buildkite_registry.test", "team_ids.#", "1"),
					),
				},
				{
					// the API now reports both teams, but team_ids stays as configured
					Config: config,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
						},
					},
					Check: resource.TestCheckResourceAttr("buildkite_registry.test", "team_ids.#", "1"),
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
						team_ids = ["%s"]
					}`, randName, os.Getenv("BUILDKITE_ORGANIZATION_SLUG"), testRegistryTeamID),
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
						rs, ok := s.RootModule().Resources["buildkite_registry.test"]
						if !ok {
							return "", fmt.Errorf("resource not found: %s", "buildkite_registry.test")
						}
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
	apiToken := os.Getenv("BUILDKITE_API_TOKEN")
	orgSlug := os.Getenv("BUILDKITE_ORGANIZATION_SLUG")
	baseURL := os.Getenv("BUILDKITE_REST_URL")
	if baseURL == "" {
		baseURL = "https://api.buildkite.com"
	}

	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_registry" {
			continue
		}

		slug := rs.Primary.Attributes["slug"]
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

		if resp.StatusCode == http.StatusNotFound {
			continue
		}

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

		if rs.Primary.Attributes["slug"] == "" {
			return fmt.Errorf("slug attribute is not set")
		}

		return nil
	}
}
