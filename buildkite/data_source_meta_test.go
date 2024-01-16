package buildkite

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Confirm that we can create a new agent token, and then delete it without error
func TestAccDataMeta_read(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccDataMetaConfigBasic(),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the meta data source has the correct values in terraform state
					testAccBuildkiteMetaCheckWebhookIps("data.buildkite_meta.foobar"),
				),
			},
		},
	})
}

func testAccBuildkiteMetaCheckWebhookIps(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]
		attributes := resourceState.Primary.Attributes

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		var (
			webhookIpsSize int
			err            error
		)
		if webhookIpsSize, err = strconv.Atoi(attributes["webhook_ips.#"]); err != nil {
			return err
		}

		if webhookIpsSize == 0 {
			return fmt.Errorf("webhook_ips attribute should have more than 0 items (len: %d)", webhookIpsSize)
		}

		return nil
	}
}

func testAccDataMetaConfigBasic() string {
	config := `
		data "buildkite_meta" "foobar" {
		}
	`
	return config
}
