package buildkite

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestAccTestSuiteTeam_add_remove(t *testing.T) {
	t.Parallel()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckTestSuiteTeamDestroy,
		Steps: []resource.TestStep{
			{

			},
		},
	})
}

func TestAccTestSuiteTeam_update(t *testing.T) {
	t.Parallel()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckTestSuiteTeamDestroy,
		Steps: []resource.TestStep{
			{

			},
		},
	})
}

func TestAccTestSuiteTeam_import(t *testing.T) {
	t.Parallel()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckTestSuiteTeamDestroy,
		Steps: []resource.TestStep{
			{

			},
		},
	})
}

func testAccCheckTestSuiteTeamDestroy(s *terraform.State) error {
	// To fill
	return nil
}
