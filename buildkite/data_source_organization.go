package buildkite

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceOrganization() *schema.Resource {
	resource := resourceOrganizationSettings()
	return &schema.Resource{
		ReadContext: resource.ReadContext,
		Schema: resource.Schema,
	}
}
