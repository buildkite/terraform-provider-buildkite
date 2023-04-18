package buildkite

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	defaultGraphqlEndpoint = "https://graphql.buildkite.com/v1"
	defaultRestEndpoint    = "https://api.buildkite.com"
)

const (
	SchemaKeyOrganization = "organization"
	SchemaKeyAPIToken     = "api_token"
	SchemaKeyGraphqlURL   = "graphql_url"
	SchemaKeyRestURL      = "rest_url"
)

// Provider creates the schema.Provider for Buildkite
func Provider(version string) *schema.Provider {
	provider := &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"buildkite_agent_token":           resourceAgentToken(),
			"buildkite_pipeline":              resourcePipeline(),
			"buildkite_pipeline_schedule":     resourcePipelineSchedule(),
			"buildkite_team":                  resourceTeam(),
			"buildkite_team_member":           resourceTeamMember(),
			"buildkite_organization_settings": resourceOrganizationSettings(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"buildkite_meta":         dataSourceMeta(),
			"buildkite_pipeline":     dataSourcePipeline(),
			"buildkite_team":         dataSourceTeam(),
			"buildkite_organization": dataSourceOrganization(),
		},
		Schema: map[string]*schema.Schema{
			SchemaKeyOrganization: {
				DefaultFunc: schema.EnvDefaultFunc("BUILDKITE_ORGANIZATION", nil),
				Description: "The Buildkite organization slug",
				Required:    true,
				Type:        schema.TypeString,
			},
			SchemaKeyAPIToken: {
				DefaultFunc: schema.EnvDefaultFunc("BUILDKITE_API_TOKEN", nil),
				Description: "API token with GraphQL access and `write_pipelines, read_pipelines` scopes",
				Required:    true,
				Type:        schema.TypeString,
			},
			SchemaKeyGraphqlURL: {
				DefaultFunc: schema.EnvDefaultFunc("BUILDKITE_GRAPHQL_URL", defaultGraphqlEndpoint),
				Description: "Base URL for the GraphQL API to use",
				Optional:    true,
				Type:        schema.TypeString,
			},
			SchemaKeyRestURL: {
				DefaultFunc: schema.EnvDefaultFunc("BUILDKITE_REST_URL", defaultRestEndpoint),
				Description: "Base URL for the REST API to use",
				Optional:    true,
				Type:        schema.TypeString,
			},
		},
	}
	provider.ConfigureFunc = providerConfigure(provider.UserAgent("buildkite", version))

	return provider
}

func providerConfigure(userAgent string) func(d *schema.ResourceData) (interface{}, error) {
	return func(d *schema.ResourceData) (interface{}, error) {
		orgName := d.Get(SchemaKeyOrganization).(string)
		apiToken := d.Get(SchemaKeyAPIToken).(string)
		graphqlUrl := d.Get(SchemaKeyGraphqlURL).(string)
		restUrl := d.Get(SchemaKeyRestURL).(string)

		config := &clientConfig{
			org:        orgName,
			apiToken:   apiToken,
			graphqlURL: graphqlUrl,
			restURL:    restUrl,
			userAgent:  userAgent,
		}

		return NewClient(config), nil
	}
}
