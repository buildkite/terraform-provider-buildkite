package buildkite

import (
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/terraform"
)

func Provider() terraform.ResourceProvider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"buildkite_agent_token": resourceAgentToken(),
			"buildkite_pipeline":    resourcePipeline(),
		},
		Schema: map[string]*schema.Schema{
			"organization": &schema.Schema{
				DefaultFunc: schema.EnvDefaultFunc("BUILDKITE_ORGANIZATION", nil),
				Description: "The Buildkite organization ID",
				Required:    true,
				Type:        schema.TypeString,
			},
			"api_token": &schema.Schema{
				DefaultFunc: schema.EnvDefaultFunc("BUILDKITE_API_TOKEN", nil),
				Description: "API token with necessary scopes", // TODO: what scopes are required?
				Required:    true,
				Type:        schema.TypeString,
			},
		},
		ConfigureFunc: providerConfigure,
	}
}

func providerConfigure(d *schema.ResourceData) (interface{}, error) {
	orgName := d.Get("organization").(string)
	apiToken := d.Get("api_token").(string)

	return NewClient(orgName, apiToken), nil
}
