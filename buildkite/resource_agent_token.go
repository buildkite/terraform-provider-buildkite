package buildkite

import (
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceAgentToken() *schema.Resource {
	return &schema.Resource{
		Create: CreateToken,
		Read:   ReadToken,
		Update: UpdateToken,
		Delete: DeleteToken,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"description": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
		},
	}
}

func CreateToken(d *schema.ResourceData, m interface{}) error {
	return nil
}

func ReadToken(d *schema.ResourceData, m interface{}) error {
	return nil
}

func UpdateToken(d *schema.ResourceData, m interface{}) error {
	return nil
}

func DeleteToken(d *schema.ResourceData, m interface{}) error {
	return nil
}
