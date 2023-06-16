package buildkite

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	framework_schema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
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

type terraformProvider struct {
	version string
}

type providerModel struct {
	api_token    *string
	graphql_url  *string
	organization *string
	rest_url     *string
}

func (tf *terraformProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var model providerModel
	diag := req.Config.Get(ctx, &model)

	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}

	legacyProvider := schema.Provider{}
	config := clientConfig{
		apiToken:   "",
		graphqlURL: defaultGraphqlEndpoint,
		org:        "",
		restURL:    defaultRestEndpoint,
		userAgent:  legacyProvider.UserAgent("buildkite", tf.version),
	}
	client, err := NewClient(&config)

	if err != nil {
		resp.Diagnostics.AddError(err.Error(), fmt.Sprintf("... details ... %s", err))
	}

	resp.ResourceData = &client
	resp.DataSourceData = &client
}

func (*terraformProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func (tf *terraformProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "buildkite"
	resp.Version = tf.version
}

func (*terraformProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{}
}

func (*terraformProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = framework_schema.Schema{
		Attributes: map[string]framework_schema.Attribute{
			SchemaKeyOrganization: framework_schema.StringAttribute{
				Optional:            true,
				Description:         "The Buildkite organization slug",
				MarkdownDescription: "The Buildkite organization slug",
			},
			SchemaKeyAPIToken: framework_schema.StringAttribute{
				Optional:            true,
				Description:         "API token with GraphQL access and `write_pipelines, read_pipelines` scopes",
				MarkdownDescription: "API token with GraphQL access and `write_pipelines, read_pipelines` scopes",
				Sensitive:           true,
			},
			SchemaKeyGraphqlURL: framework_schema.StringAttribute{
				Optional:            true,
				Description:         "Base URL for the GraphQL API to use",
				MarkdownDescription: "Base URL for the GraphQL API to use",
			},
			SchemaKeyRestURL: framework_schema.StringAttribute{
				Optional:            true,
				Description:         "Base URL for the REST API to use",
				MarkdownDescription: "Base URL for the REST API to use",
			},
		},
	}
}

func New(version string) provider.Provider {
	return &terraformProvider{
		version: version,
	}
}

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

		return NewClient(config)
	}
}
