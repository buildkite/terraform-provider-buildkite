package buildkite

import (
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/terraform"
)

func Provider() terraform.ResourceProvider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"agent_token": resourceAgentToken(),
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
	}
}
