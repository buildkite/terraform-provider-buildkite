package buildkite

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const graphqlEndpoint = "https://graphql.buildkite.com/v1"
const restEndpoint = "https://api.buildkite.com"

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
			"graphql_url": &schema.Schema{
				DefaultFunc: schema.EnvDefaultFunc("BUILDKITE_GRAPHQL_URL", graphqlEndpoint),
				Description: "Base URL for the GraphQL API to use",
				Optional:    true,
				Type:        schema.TypeString,
			},
			"rest_url": &schema.Schema{
				DefaultFunc: schema.EnvDefaultFunc("BUILDKITE_REST_URL", restEndpoint),
				Description: "Base URL for the REST API to use",
				Optional:    true,
				Type:        schema.TypeString,
			},
		},
		ConfigureFunc: providerConfigure,
	}
}

func providerConfigure(d *schema.ResourceData) (interface{}, error) {
	orgName := d.Get("organization").(string)
	apiToken := d.Get("api_token").(string)
	graphqlUrl := d.Get("graphql_url").(string)
	restUrl := d.Get("rest_url").(string)

	return NewClient(orgName, apiToken, graphqlUrl, restUrl), nil
}
