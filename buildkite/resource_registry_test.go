package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
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

func TestAccBuildkiteRegistryPermissions(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}
	testAccPreCheck(t)
	memberToken := os.Getenv("BUILDKITE_NONADMIN_API_TOKEN")
	if memberToken == "" {
		t.Fatal("BUILDKITE_NONADMIN_API_TOKEN must contain the member token for registry tests; BUILDKITE_API_TOKEN must remain the admin token")
	}
	admin := getTestClient()
	member := getTestClientWithToken(memberToken)
	testAccMemberPreCheck(t, member.graphql)
	t.Setenv("TF_VAR_registry_member_token", memberToken)
	const address = "buildkite_registry.temporary"
	const memberProvider = `
		variable "registry_member_token" {
			type = string
			sensitive = true
		}
		provider "buildkite" {
			api_token = var.registry_member_token
		}`
	fixtures := fmt.Sprintf(`
		data "buildkite_team" "owner" {
			slug = %q
		}`, testAccFixturePipelineTeamSlug)
	const registryDataSource = `
		data "buildkite_registry" "temporary" {
			slug = buildkite_registry.temporary.slug
		}`
	config := func(name, description, emoji, color string) string {
		return fixtures + fmt.Sprintf(`
			resource "buildkite_registry" "temporary" {
				name = %q
				ecosystem = "python"
				description = %q
				emoji = %q
				color = %q
				team_ids = [data.buildkite_team.owner.uuid]
			}`, name, description, emoji, color)
	}

	t.Run("member_create", func(t *testing.T) {
		name := testAccResourceName() + "-nonadmincreate"
		const description = "Temporary member-created registry"
		registryConfig := config(name, description, ":snake:", "#3776AB")
		if err := testAccCheckRegistryDeleted(admin, name); err != nil {
			t.Fatal(err)
		}
		// Independent admin state owns cleanup even if the member test fails
		// before forgetting its resource or recording a successful create.
		t.Cleanup(func() {
			var remote registryAPIResponse
			path := fmt.Sprintf("/v2/packages/organizations/%s/registries/%s", admin.organization, name)
			err := admin.makeRequest(context.Background(), http.MethodGet, path, nil, &remote)
			if isAPIStatus(err, http.StatusNotFound) {
				return
			}
			if err != nil {
				t.Fatalf("read member-created registry for admin cleanup: %v", err)
			}
			if remote.Name != name || remote.Slug != name {
				t.Fatal("refusing cleanup of a registry that does not match this test's unique name and slug")
			}
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             testAccCheckRegistryDestroy,
				Steps: []resource.TestStep{{
					Config:             registryConfig,
					ResourceName:       address,
					ImportState:        true,
					ImportStateId:      name,
					ImportStatePersist: true,
				}},
			})
			if err := testAccCheckRegistryDeleted(admin, name); err != nil {
				t.Error(err)
			}
		})
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: memberProvider + registryConfig + registryDataSource,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr(address, "name", name),
						resource.TestCheckResourceAttr(address, "ecosystem", "python"),
						resource.TestCheckResourceAttr(address, "description", description),
						resource.TestCheckResourceAttr(address, "public", "false"),
						resource.TestCheckResourceAttrSet(address, "id"),
						resource.TestCheckResourceAttrSet(address, "uuid"),
						resource.TestCheckResourceAttr(address, "slug", name),
						resource.TestCheckResourceAttrPair(address, "team_ids.0", "data.buildkite_team.owner", "uuid"),
						resource.TestCheckResourceAttrPair("data.buildkite_registry.temporary", "id", address, "id"),
					),
				},
				{
					Config: memberProvider + fixtures + `
						removed {
							from = buildkite_registry.temporary
							lifecycle { destroy = false }
						}`,
					Check: func(s *terraform.State) error {
						if _, ok := s.RootModule().Resources[address]; ok {
							return fmt.Errorf("member-created registry remains in member state")
						}
						var remote registryAPIResponse
						path := fmt.Sprintf("/v2/packages/organizations/%s/registries/%s", member.organization, name)
						if err := member.makeRequest(context.Background(), http.MethodGet, path, nil, &remote); err != nil {
							return err
						}
						if remote.Name != name || remote.Slug != name {
							return fmt.Errorf("forgetting member state must leave its temporary registry intact for admin cleanup")
						}
						return nil
					},
				},
			},
		})
	})

	t.Run("delegated_lifecycle", func(t *testing.T) {
		name := testAccResourceName() + "-delegatedregistry"
		initial := config(name, "Initial registry description", ":snake:", "#3776AB") + registryDataSource
		adminUpdated := config(name, "Admin registry update", ":package:", "#123456")
		memberUpdated := config(name, "Member registry update", ":buildkite:", "#00FF00") + registryDataSource
		const ownerConfig = `
			resource "buildkite_team_registry" "owner" {
				registry_id = buildkite_registry.temporary.id
				team_id = data.buildkite_team.owner.id
				access_level = "READ_WRITE_AND_ADMIN"
			}`
		var registryID, registryUUID, slug, ownerID, ownerLinkID string
		readOwner := func(client *Client, requireAdmin bool) error {
			var query struct {
				Node struct {
					Registry struct {
						Teams struct {
							Edges []struct {
								Node struct {
									ID          string
									AccessLevel graphql.String
									Team        struct{ ID string }
								}
							}
							PageInfo struct{ HasNextPage graphql.Boolean }
						} `graphql:"teams(first: 100)"`
					} `graphql:"... on Registry"`
				} `graphql:"node(id: $id)"`
			}
			ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
			defer cancel()
			if err := client.graphql.Query(ctx, &query, map[string]any{"id": graphql.ID(registryID)}); err != nil {
				return err
			}
			teams := query.Node.Registry.Teams
			if teams.PageInfo.HasNextPage {
				return fmt.Errorf("registry teams query was truncated")
			}
			for _, edge := range teams.Edges {
				if edge.Node.Team.ID != ownerID {
					continue
				}
				if edge.Node.ID == "" || (ownerLinkID != "" && edge.Node.ID != ownerLinkID) {
					return fmt.Errorf("registry owner association identity changed")
				}
				ownerLinkID = edge.Node.ID
				if requireAdmin && edge.Node.AccessLevel != "READ_WRITE_AND_ADMIN" {
					return fmt.Errorf("registry owner lost Full Access: %s", edge.Node.AccessLevel)
				}
				return nil
			}
			return fmt.Errorf("registry owner association is missing")
		}
		check := func(client *Client, description, emoji, color string, requireAdmin bool) resource.TestCheckFunc {
			return func(s *terraform.State) error {
				for _, key := range []string{address, "data.buildkite_team.owner"} {
					if rs, ok := s.RootModule().Resources[key]; !ok || rs.Primary == nil || rs.Primary.ID == "" {
						return fmt.Errorf("%s is missing from Terraform state", key)
					}
				}
				state := s.RootModule().Resources[address].Primary
				owner := s.RootModule().Resources["data.buildkite_team.owner"].Primary
				if registryID == "" {
					registryID, registryUUID, slug, ownerID = state.ID, state.Attributes["uuid"], state.Attributes["slug"], owner.ID
				}
				checks := []resource.TestCheckFunc{
					resource.TestCheckResourceAttr(address, "id", registryID),
					resource.TestCheckResourceAttrSet(address, "uuid"),
					resource.TestCheckResourceAttr(address, "uuid", registryUUID),
					resource.TestCheckResourceAttr(address, "slug", slug),
					resource.TestCheckResourceAttr(address, "name", name),
					resource.TestCheckResourceAttr(address, "ecosystem", "python"),
					resource.TestCheckResourceAttr(address, "description", description),
					resource.TestCheckResourceAttr(address, "emoji", emoji),
					resource.TestCheckResourceAttr(address, "color", color),
					resource.TestCheckResourceAttr(address, "public", "false"),
					resource.TestCheckResourceAttr(address, "registry_type", "source"),
					resource.TestCheckResourceAttr(address, "team_ids.#", "1"),
					resource.TestCheckResourceAttrPair(address, "team_ids.0", "data.buildkite_team.owner", "uuid"),
				}
				for _, field := range []string{"id", "uuid", "slug", "name", "ecosystem", "description", "emoji", "color", "public", "registry_type", "team_ids.#", "team_ids.0"} {
					checks = append(checks, resource.TestCheckResourceAttrPair("data.buildkite_registry.temporary", field, address, field))
				}
				if err := resource.ComposeAggregateTestCheckFunc(checks...)(s); err != nil {
					return err
				}
				var remote registryAPIResponse
				path := fmt.Sprintf("/v2/packages/organizations/%s/registries/%s", client.organization, slug)
				if err := client.makeRequest(context.Background(), http.MethodGet, path, nil, &remote); err != nil {
					return err
				}
				if remote.GraphQLID != registryID || remote.ID != registryUUID || remote.Slug != slug || remote.Name != name || remote.Ecosystem != "python" || remote.Public || remote.RegistryType != "source" ||
					remote.Description == nil || *remote.Description != description || remote.Emoji == nil || *remote.Emoji != emoji || remote.Color == nil || *remote.Color != color ||
					len(remote.TeamIDs) != 1 || remote.TeamIDs[0] != owner.Attributes["uuid"] {
					return fmt.Errorf("remote registry does not match the expected identity, settings, and team")
				}
				return readOwner(client, requireAdmin)
			}
		}
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckRegistryDestroy,
			Steps: []resource.TestStep{
				{Config: initial, Check: check(admin, "Initial registry description", ":snake:", "#3776AB", false)},
				{
					Config:             initial + ownerConfig,
					ResourceName:       "buildkite_team_registry.owner",
					ImportState:        true,
					ImportStatePersist: true,
					ImportStateIdFunc:  func(*terraform.State) (string, error) { return ownerLinkID, nil },
				},
				{Config: initial + ownerConfig, Check: check(admin, "Initial registry description", ":snake:", "#3776AB", true)},
				{
					Config: adminUpdated + ownerConfig + registryDataSource,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
					},
					Check: check(admin, "Admin registry update", ":package:", "#123456", true),
				},
				{
					ResourceName:      address,
					ImportState:       true,
					ImportStateIdFunc: func(*terraform.State) (string, error) { return slug, nil },
					ImportStateVerify: true,
				},
				{
					Config: adminUpdated + ownerConfig,
					// The member deletes the registry during Check; the admin state
					// must notice its disappearance without applying a recreation.
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply:             []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
						PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionCreate)},
					},
					Check: func(s *terraform.State) error {
						registry, ok := s.RootModule().Resources[address]
						if !ok || registry.Primary == nil {
							return fmt.Errorf("admin registry is missing from Terraform state")
						}
						attrs := registry.Primary.Attributes
						if !t.Run("member_lifecycle", func(t *testing.T) {
							resource.Test(t, resource.TestCase{
								ProtoV6ProviderFactories: protoV6ProviderFactories(),
								CheckDestroy:             testAccCheckRegistryDestroy,
								Steps: []resource.TestStep{
									{
										Config:             memberProvider + adminUpdated + registryDataSource,
										ResourceName:       address,
										ImportState:        true,
										ImportStateId:      slug,
										ImportStatePersist: true,
										ImportStateCheck: func(states []*terraform.InstanceState) error {
											if len(states) != 1 || states[0].ID != registryID {
												return fmt.Errorf("member import did not return the temporary registry")
											}
											for _, field := range []string{"id", "uuid", "slug", "name", "ecosystem", "description", "emoji", "color", "public", "registry_type", "team_ids.#", "team_ids.0"} {
												if states[0].Attributes[field] != attrs[field] {
													return fmt.Errorf("member import changed registry %s", field)
												}
											}
											return nil
										},
									},
									{
										Config:           memberProvider + adminUpdated + registryDataSource,
										ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
										Check:            check(member, "Admin registry update", ":package:", "#123456", true),
									},
									{
										Config:           memberProvider + memberUpdated,
										ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)}},
										Check:            check(member, "Member registry update", ":buildkite:", "#00FF00", true),
									},
									{RefreshState: true, Check: check(member, "Member registry update", ":buildkite:", "#00FF00", true)},
									{ResourceName: address, ImportState: true, ImportStateId: slug, ImportStateVerify: true},
									{
										Config:           memberProvider + fixtures,
										ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroy)}},
										Check: func(s *terraform.State) error {
											if _, ok := s.RootModule().Resources[address]; ok {
												return fmt.Errorf("deleted registry remains in member state")
											}
											if err := testAccCheckRegistryDeleted(member, slug); err != nil {
												return err
											}
											return testAccCheckRegistryDeleted(admin, slug)
										},
									},
								},
							})
						}) {
							return fmt.Errorf("member registry lifecycle failed; independent admin state retains cleanup ownership")
						}
						return nil
					},
				},
			},
		})
	})
}

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
		randName := testAccResourceName()
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

	t.Run("update", func(t *testing.T) {
		randName := testAccResourceName()
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
		randName := testAccResourceName()

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
		randName := testAccResourceName()

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
		randName := testAccResourceName()

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
		randName := testAccResourceName()

		config := fmt.Sprintf(`
			provider "buildkite" {}

			resource "buildkite_team" "owner" {
				name = "%s-registry-owner"
				privacy = "VISIBLE"
				default_team = false
				default_member_role = "MEMBER"
			}

			resource "buildkite_team" "other" {
				name = "%s-registry-other"
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
			ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
			defer cancel()
			return graphqlClient.Mutate(ctx, &mutation, map[string]interface{}{
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
		randName := testAccResourceName()

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
		randName := testAccResourceName()
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
	for name, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_registry" || strings.HasPrefix(name, "data.") {
			continue
		}

		if err := testAccCheckRegistryDeleted(getTestClient(), rs.Primary.Attributes["slug"]); err != nil {
			return err
		}
	}

	return nil
}

func testAccCheckRegistryDeleted(client *Client, slug string) error {
	path := fmt.Sprintf("/v2/packages/organizations/%s/registries/%s", client.organization, slug)
	var result struct{}
	err := client.makeRequest(context.Background(), http.MethodGet, path, nil, &result)
	if isAPIStatus(err, http.StatusNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking registry deletion: %w", err)
	}
	return fmt.Errorf("buildkite_registry resource still exists: %s", slug)
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
