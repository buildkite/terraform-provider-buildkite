package buildkite

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider creates the schema.Provider for Buildkite
func Provider() *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"buildkite_agent_token":       resourceAgentToken(),
			"buildkite_pipeline":          resourcePipeline(),
			"buildkite_pipeline_schedule": resourcePipelineSchedule(),
			"buildkite_team":              resourceTeam(),
			"buildkite_team_member":       resourceTeamMember(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"buildkite_meta":     dataSourceMeta(),
			"buildkite_pipeline": dataSourcePipeline(),
			"buildkite_team":     dataSourceTeam(),
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
				Description: "API token with GraphQL access and `write_pipelines, read_pipelines` scopes",
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
