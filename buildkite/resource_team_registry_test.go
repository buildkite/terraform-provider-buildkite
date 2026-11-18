package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/shurcooL/graphql"
)

func TestAccBuildkiteTeamRegistryPermissions(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}
	testAccPreCheck(t)
	memberToken := os.Getenv("BUILDKITE_NONADMIN_API_TOKEN")
	if memberToken == "" {
		t.Fatal("BUILDKITE_NONADMIN_API_TOKEN must contain the member token; BUILDKITE_API_TOKEN must remain the admin token")
	}
	admin := getTestClient()
	member := getTestClientWithToken(memberToken)
	testAccMemberPreCheck(t, member.graphql)
	t.Setenv("TF_VAR_registry_member_token", memberToken)
	secondarySlug := os.Getenv("BUILDKITE_NONADMIN_TEAM_SLUG")
	if secondarySlug == "" {
		secondarySlug = "everyone"
	}
	if secondarySlug == testAccFixturePipelineTeamSlug {
		t.Fatal("secondary registry team must differ from its owner")
	}
	const address = "buildkite_team_registry.secondary"
	fixtures := fmt.Sprintf(`
		data "buildkite_team" "owner" { slug = %q }
		data "buildkite_team" "secondary" { slug = %q }
	`, testAccFixturePipelineTeamSlug, secondarySlug)
	name := testAccResourceName() + "-nonadminregistryteam"
	baseConfig := fixtures + fmt.Sprintf(`
		resource "buildkite_registry" "temporary" {
			name = %q
			ecosystem = "python"
			team_ids = [data.buildkite_team.owner.uuid]
		}
	`, name)
	const ownerConfig = `
		resource "buildkite_team_registry" "owner" {
			registry_id = buildkite_registry.temporary.id
			team_id = data.buildkite_team.owner.id
			access_level = "READ_WRITE_AND_ADMIN"
		}`
	var registryID, registryUUID, slug, ownerID, secondaryID, ownerLinkID, associationID, associationUUID string
	readTeams := func(client *Client, access string, requireFullAccess bool) error {
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
		var ownerFound, secondaryFound bool
		for _, edge := range teams.Edges {
			switch edge.Node.Team.ID {
			case ownerID:
				ownerFound = true
				if edge.Node.ID == "" || (ownerLinkID != "" && edge.Node.ID != ownerLinkID) {
					return fmt.Errorf("registry owner association identity changed")
				}
				ownerLinkID = edge.Node.ID
				if requireFullAccess && edge.Node.AccessLevel != "READ_WRITE_AND_ADMIN" {
					return fmt.Errorf("registry owner lost Full Access")
				}
			case secondaryID:
				secondaryFound = true
				if edge.Node.ID != associationID || string(edge.Node.AccessLevel) != access {
					return fmt.Errorf("secondary registry team has unexpected identity or access")
				}
			}
		}
		if !ownerFound || secondaryFound != (access != "") {
			return fmt.Errorf("registry owner present: %t; secondary team present: %t (expected access %q)", ownerFound, secondaryFound, access)
		}
		var remote registryAPIResponse
		path := fmt.Sprintf("/v2/packages/organizations/%s/registries/%s", client.organization, slug)
		if err := client.makeRequest(ctx, http.MethodGet, path, nil, &remote); err != nil {
			return err
		}
		if remote.GraphQLID != registryID || remote.ID != registryUUID || remote.Slug != slug || remote.Name != name || remote.Ecosystem != "python" || remote.Public {
			return fmt.Errorf("temporary registry identity, settings, or private visibility changed")
		}
		return nil
	}
	bootstrapCheck := func(requireFullAccess bool) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			for _, key := range []string{"buildkite_registry.temporary", "data.buildkite_team.owner", "data.buildkite_team.secondary"} {
				if rs, ok := s.RootModule().Resources[key]; !ok || rs.Primary == nil || rs.Primary.ID == "" {
					return fmt.Errorf("%s is missing from Terraform state", key)
				}
			}
			registry := s.RootModule().Resources["buildkite_registry.temporary"].Primary
			owner := s.RootModule().Resources["data.buildkite_team.owner"].Primary
			secondary := s.RootModule().Resources["data.buildkite_team.secondary"].Primary
			if owner.ID == secondary.ID {
				return fmt.Errorf("secondary registry team must differ from its owner")
			}
			if registryID == "" {
				registryID, registryUUID, slug, ownerID, secondaryID = registry.ID, registry.Attributes["uuid"], registry.Attributes["slug"], owner.ID, secondary.ID
			}
			return readTeams(admin, "", requireFullAccess)
		}
	}
	check := func(client *Client, access string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			if access == "" {
				if _, exists := s.RootModule().Resources[address]; exists {
					return fmt.Errorf("removed registry-team association remains in Terraform state")
				}
			} else {
				association, ok := s.RootModule().Resources[address]
				if !ok || association.Primary == nil || association.Primary.ID == "" {
					return fmt.Errorf("secondary registry-team association is missing from Terraform state")
				}
				if associationID == "" {
					associationID, associationUUID = association.Primary.ID, association.Primary.Attributes["uuid"]
				}
				if err := resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", associationID),
					resource.TestCheckResourceAttrSet(address, "uuid"),
					resource.TestCheckResourceAttr(address, "uuid", associationUUID),
					resource.TestCheckResourceAttr(address, "registry_id", registryID),
					resource.TestCheckResourceAttr(address, "team_id", secondaryID),
					resource.TestCheckResourceAttr(address, "access_level", access),
				)(s); err != nil {
					return err
				}
			}
			node, err := getNode(context.Background(), client.genqlient, associationID)
			if err != nil {
				return err
			}
			if access == "" {
				if node.Node != nil {
					return fmt.Errorf("removed registry-team association is still readable")
				}
			} else {
				link, ok := node.Node.(*getNodeNodeTeamRegistry)
				if !ok || link.Id != associationID || link.TeamRegistryUuid != associationUUID || link.Team.Id != secondaryID || link.Registry.Id != registryID || string(link.RegistryAccessLevel) != access {
					return fmt.Errorf("remote registry-team association does not match its expected identity, endpoints, and access")
				}
			}
			return readTeams(client, access, true)
		}
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckRegistryDestroy,
		Steps: []resource.TestStep{
			{Config: baseConfig, Check: bootstrapCheck(false)},
			{
				Config:             baseConfig + ownerConfig,
				ResourceName:       "buildkite_team_registry.owner",
				ImportState:        true,
				ImportStatePersist: true,
				ImportStateIdFunc:  func(*terraform.State) (string, error) { return ownerLinkID, nil },
			},
			{
				Config: baseConfig + ownerConfig,
				Check: func(s *terraform.State) error {
					if err := bootstrapCheck(true)(s); err != nil {
						return err
					}
					const memberProvider = `
						variable "registry_member_token" {
							type = string
							sensitive = true
						}
						provider "buildkite" { api_token = var.registry_member_token }
					`
					lookupConfig := fixtures + fmt.Sprintf(`
						data "buildkite_registry" "temporary" { slug = %q }
					`, slug)
					for _, actor := range []struct {
						name     string
						client   *Client
						provider string
					}{
						{name: "admin", client: admin},
						{name: "member", client: member, provider: memberProvider},
					} {
						associationID, associationUUID = "", ""
						actorBase := actor.provider + lookupConfig
						config := func(access string) string {
							return actorBase + fmt.Sprintf(`
							resource "buildkite_team_registry" "secondary" {
								registry_id = data.buildkite_registry.temporary.id
								team_id = data.buildkite_team.secondary.id
								access_level = %q
							}
						`, access)
						}
						updateChecks := resource.ConfigPlanChecks{
							PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
							PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
						}
						if !t.Run(actor.name+"_lifecycle", func(t *testing.T) {
							resource.Test(t, resource.TestCase{
								ProtoV6ProviderFactories: protoV6ProviderFactories(),
								CheckDestroy:             testAccCheckTeamRegistryDestroy,
								Steps: []resource.TestStep{
									{Config: config("READ_ONLY"), Check: check(actor.client, "READ_ONLY")},
									{Config: config("READ_AND_WRITE"), Check: check(actor.client, "READ_AND_WRITE"), ConfigPlanChecks: updateChecks},
									{ResourceName: address, ImportState: true, ImportStateVerify: true},
									{Config: config("READ_WRITE_AND_ADMIN"), Check: check(actor.client, "READ_WRITE_AND_ADMIN"), ConfigPlanChecks: updateChecks},
									{RefreshState: true, Check: check(actor.client, "READ_WRITE_AND_ADMIN")},
									{
										Config: actorBase,
										Check:  check(actor.client, ""),
										ConfigPlanChecks: resource.ConfigPlanChecks{
											PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroy)},
											PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
										},
									},
								},
							})
						}) {
							return fmt.Errorf("%s registry-team lifecycle failed; independent admin state retains registry cleanup ownership", actor.name)
						}
						if err := readTeams(admin, "", true); err != nil {
							return err
						}
					}
					return nil
				},
			},
		},
	})
}

func TestAccBuildkiteTeamRegistryResource(t *testing.T) {
	t.Parallel()
	config := func(name, accessLevel string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_team" "ownerteam" {
			name = "%s-owner-team"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}

		resource "buildkite_team" "newteam" {
			name = "%s-new-team"
			default_team = false
			privacy = "VISIBLE"
			default_member_role = "MAINTAINER"
		}

		resource "buildkite_registry" "registry" {
			name = "%s-registry"
			ecosystem = "java"
			team_ids = [buildkite_team.ownerteam.uuid]
		}

		resource "buildkite_team_registry" "teamregistry" {
			registry_id = buildkite_registry.registry.id
			team_id = buildkite_team.newteam.id
			access_level = "%s"
		}
		`, name, name, name, accessLevel)
	}

	t.Run("team registry is recreated if removed", func(t *testing.T) {
		name := testAccResourceName()

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckTeamRegistryDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(name, "READ_ONLY"),
					Check:  testAccCheckTeamRegistryDisappears("buildkite_team_registry.teamregistry"),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							// expect terraform to plan a new create
							plancheck.ExpectResourceAction("buildkite_team_registry.teamregistry", plancheck.ResourceActionCreate),
						},
					},
					ExpectNonEmptyPlan: true,
				},
			},
		})
	})
}

func testAccCheckTeamRegistryDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_team_registry" {
			continue
		}

		apiResponse, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("Error fetching team registry from graphql API: %v", err)
		}

		if teamRegistryNode, ok := apiResponse.GetNode().(*getNodeNodeTeamRegistry); ok {
			if teamRegistryNode != nil {
				return fmt.Errorf("Team registry still exists")
			}
		}
	}
	return nil
}

func testAccCheckTeamRegistryDisappears(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Resource not found: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("Resource ID missing: %s", resourceName)
		}

		_, err := deleteTeamRegistry(context.Background(), genqlientGraphql, resourceState.Primary.ID)

		return err
	}
}
