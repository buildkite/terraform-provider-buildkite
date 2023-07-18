package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func testAccTeamConfigBasic(name string) string {
	config := `
		resource "buildkite_team" "test" {
		  name = "%s"
			description = "a cool team of %s"
		  privacy = "VISIBLE"
		  default_team = true
		  default_member_role = "MEMBER"
		}
	`
	return fmt.Sprintf(config, name, name)
}

func testAccTeamConfigSecret(name string) string {
	config := `
		resource "buildkite_team" "test" {
		  name = "%s"
			description = "a cool team of %s"
		  privacy = "SECRET"
		  default_team = true
		  default_member_role = "MEMBER"
		}
	`
	return fmt.Sprintf(config, name, name)
}

func TestAccTeam_AddRemove(t *testing.T) {
	t.Parallel()
	var tr teamResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckTeamResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamConfigBasic("developers"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTeamExists("buildkite_team.test", &tr),
					testAccCheckTeamRemoteValues("developers", &tr),
					resource.TestCheckResourceAttr("buildkite_team.test", "name", "developers"),
					resource.TestCheckResourceAttr("buildkite_team.test", "privacy", "VISIBLE"),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_team.test", "name"),
				),
			},
		},
	})
}

func TestAccTeam_Update(t *testing.T) {
	t.Parallel()
	var rt = new(teamResourceModel)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckTeamResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccTeamConfigBasic("developers"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTeamExists("buildkite_team.test", rt),
					resource.TestCheckResourceAttr("buildkite_team.test", "name", "developers"),
				),
			},
			{
				Config: testAccTeamConfigBasic("wombats"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTeamExists("buildkite_team.test", rt),
					testAccCheckTeamRemoteValues("wombats", rt),
					resource.TestCheckResourceAttr("buildkite_team.test", "name", "wombats"),
					resource.TestCheckResourceAttr("buildkite_team.test", "name", "wombats"),
				),
			},
			{
				Config: testAccTeamConfigSecret("wombats"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTeamExists("buildkite_team.test", rt),
					testAccCheckTeamRemoteValues("wombats", rt),
					resource.TestCheckResourceAttr("buildkite_team.test", "name", "wombats"),
					resource.TestCheckResourceAttr("buildkite_team.test", "description", "a secret team of wombats"),
					resource.TestCheckResourceAttr("buildkite_team.test", "privacy", "SECRET"),
				),
			},
		},
	})
}

func testAccCheckTeamExists(name string, tr *teamResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]

		if !ok {
			return fmt.Errorf("Not found in state: %s", name)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		r, err := getTeam(genqlientGraphql, rs.Primary.ID)

		if err != nil {
			return err
		}

		updateTeamResourceState(tr, r.GetNode().(*getTeamNodeTeam))
		return nil
	}
}

func testAccCheckTeamRemoteValues(name string, tr *teamResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if tr.Name.ValueString() != name {
			return fmt.Errorf("remote team name (%s) doesn't match expected value (%s)", tr.Name, name)
		}
		return nil
	}
}

func testAccCheckTeamResourceDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_team" {
			continue
		}

		_, err := getTeam(genqlientGraphql, rs.Primary.ID)

		if err == nil {
			return fmt.Errorf("Team still exists")
		}
		return nil
	}
	return nil
}
