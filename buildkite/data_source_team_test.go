package buildkite

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func testDatasourceTeamConfigID(name string) string {
	data := `
		resource "buildkite_team" "acc_tests_id" {
			name = "%s"
			description = "a cool team of %s"
			privacy = "VISIBLE"
			default_team = true
			default_member_role = "MEMBER"
			members_can_create_pipelines = true
		}

		data "buildkite_team" "data_test_id" {
			depends_on = [buildkite_team.acc_tests_id]
			id = buildkite_team.acc_tests_id.id
		}
	`
	return fmt.Sprintf(data, name, name)
}

func testDatasourceTeamConfigSlug(name string) string {
	data := `
		resource "buildkite_team" "acc_tests_slug" {
			name = "%s"
			description = "a cool team of %s"
			privacy = "VISIBLE"
			default_team = true
			default_member_role = "MEMBER"
		}

		data "buildkite_team" "data_test_slug" {
			depends_on = [buildkite_team.acc_tests_slug]
			slug = buildkite_team.acc_tests_slug.slug
		}
	`
	return fmt.Sprintf(data, name, name)
}

func testDatasourceTeamConfigIDSlug(name string) string {
	data := `
		resource "buildkite_team" "acc_tests_slugid" {
			name = "%s"
			description = "a cool team of %s"
			privacy = "VISIBLE"
			default_team = true
			default_member_role = "MEMBER"
		}

		data "buildkite_team" "data_test_slug" {
			depends_on = [buildkite_team.acc_tests_slugid]
			id = buildkite_team.acc_tests_slugid.id
			slug = buildkite_team.acc_tests_slugid.slug
		}
	`
	return fmt.Sprintf(data, name, name)
}

func testDatasourceTeamConfigWithAllPermissions(name string) string {
	data := `
		resource "buildkite_team" "acc_tests_perms" {
			name = "%s"
			description = "team with all permissions"
			privacy = "VISIBLE"
			default_team = false
			default_member_role = "MEMBER"
			members_can_create_pipelines = true
			members_can_create_suites = true
			members_can_create_registries = true
			members_can_destroy_registries = true
			members_can_destroy_packages = true
		}

		data "buildkite_team" "data_test_perms" {
			depends_on = [buildkite_team.acc_tests_perms]
			id = buildkite_team.acc_tests_perms.id
		}
	`
	return fmt.Sprintf(data, name)
}

func TestAccDataTeam_ReadUsingSlug(t *testing.T) {
	t.Parallel()
	var tr teamResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testDatasourceTeamConfigSlug("designers"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTeamExists("buildkite_team.acc_tests_slug", &tr),
					testAccCheckTeamRemoteValues("designers", &tr),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_slug", "name", "designers"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_slug", "members_can_create_pipelines", "false"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_slug", "members_can_create_suites", "false"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_slug", "members_can_create_registries", "false"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_slug", "members_can_destroy_registries", "false"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_slug", "members_can_destroy_packages", "false"),
				),
			},
		},
	})
}

func TestAccDataTeam_ReadUsingID(t *testing.T) {
	t.Parallel()
	var tr teamResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testDatasourceTeamConfigID("developers"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTeamExists("buildkite_team.acc_tests_id", &tr),
					testAccCheckTeamRemoteValues("developers", &tr),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_id", "name", "developers"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_id", "members_can_create_pipelines", "true"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_id", "members_can_create_suites", "false"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_id", "members_can_create_registries", "false"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_id", "members_can_destroy_registries", "false"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_id", "members_can_destroy_packages", "false"),
				),
			},
		},
	})
}

func TestAccDataTeam_ReadUsingIDAndSlug(t *testing.T) {
	t.Parallel()
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Config:                   testDatasourceTeamConfigIDSlug("noobs"),
				ExpectError:              regexp.MustCompile("Invalid Attribute Combination"),
			},
		},
	})
}

func TestAccDataTeam_ReadWithAllPermissions(t *testing.T) {
	t.Parallel()
	var tr teamResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testDatasourceTeamConfigWithAllPermissions("full-access-team"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTeamExists("buildkite_team.acc_tests_perms", &tr),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_perms", "name", "full-access-team"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_perms", "members_can_create_pipelines", "true"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_perms", "members_can_create_suites", "true"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_perms", "members_can_create_registries", "true"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_perms", "members_can_destroy_registries", "true"),
					resource.TestCheckResourceAttr("data.buildkite_team.data_test_perms", "members_can_destroy_packages", "true"),
				),
			},
		},
	})
}
