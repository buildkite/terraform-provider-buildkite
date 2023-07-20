package buildkite

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	framework_schema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
	ApiToken     types.String `tfsdk:"api_token"`
	GraphqlUrl   types.String `tfsdk:"graphql_url"`
	Organization types.String `tfsdk:"organization"`
	RestUrl      types.String `tfsdk:"rest_url"`
}

func (tf *terraformProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	apiToken := os.Getenv("BUILDKITE_API_TOKEN")
	graphqlUrl := defaultGraphqlEndpoint
	organization := getenv("BUILDKITE_ORGANIZATION_SLUG")
	restUrl := defaultRestEndpoint

	if data.ApiToken.ValueString() != "" {
		apiToken = data.ApiToken.ValueString()
	}
	if data.GraphqlUrl.ValueString() != "" {
		graphqlUrl = data.GraphqlUrl.ValueString()
	} else if v, ok := os.LookupEnv("BUILDKITE_GRAPHQL_URL"); ok {
		graphqlUrl = v
	}
	if data.Organization.ValueString() != "" {
		organization = data.Organization.ValueString()
	}
	if data.RestUrl.ValueString() != "" {
		restUrl = data.RestUrl.ValueString()
	} else if v, ok := os.LookupEnv("BUILDKITE_REST_URL"); ok {
		restUrl = v
	}

	legacyProvider := schema.Provider{}
	config := clientConfig{
		apiToken:   apiToken,
		graphqlURL: graphqlUrl,
		org:        organization,
		restURL:    restUrl,
		userAgent:  legacyProvider.UserAgent("buildkite", tf.version),
	}
	client, err := NewClient(&config)

	if err != nil {
		resp.Diagnostics.AddError(err.Error(), fmt.Sprintf("... details ... %s", err))
	}

	resp.ResourceData = client
	resp.DataSourceData = client
}

func (*terraformProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newClusterDatasource,
		newOrganizationDatasource,
		newMetaDatasource,
		newPipelineDatasource,
	}
}

func (tf *terraformProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "buildkite"
	resp.Version = tf.version
}

func (*terraformProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newAgentTokenResource,
		newClusterAgentTokenResource,
		NewClusterQueueResource,
		newClusterResource,
		newTeamMemberResource,
		newOrganizationResource,
	}
}

func (*terraformProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = framework_schema.Schema{
		Attributes: map[string]framework_schema.Attribute{
			SchemaKeyOrganization: framework_schema.StringAttribute{
				Optional:    true,
				Description: "The Buildkite organization slug",
			},
			SchemaKeyAPIToken: framework_schema.StringAttribute{
				Optional:    true,
				Description: "API token with GraphQL access and `write_pipelines, read_pipelines` scopes",
				Sensitive:   true,
			},
			SchemaKeyGraphqlURL: framework_schema.StringAttribute{
				Optional:    true,
				Description: "Base URL for the GraphQL API to use",
			},
			SchemaKeyRestURL: framework_schema.StringAttribute{
				Optional:    true,
				Description: "Base URL for the REST API to use",
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
			"buildkite_pipeline":              resourcePipeline(),
			"buildkite_pipeline_schedule":     resourcePipelineSchedule(),
			"buildkite_team":                  resourceTeam(),
			"buildkite_organization_settings": resourceOrganizationSettings(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"buildkite_team": dataSourceTeam(),
		},
		Schema: map[string]*schema.Schema{
			SchemaKeyOrganization: {
				Description: "The Buildkite organization slug",
				Optional:    true,
				Type:        schema.TypeString,
			},
			SchemaKeyAPIToken: {
				Description: "API token with GraphQL access and `write_pipelines, read_pipelines` scopes",
				Optional:    true,
				Type:        schema.TypeString,
				Sensitive:   true,
			},
			SchemaKeyGraphqlURL: {
				Description: "Base URL for the GraphQL API to use",
				Optional:    true,
				Type:        schema.TypeString,
			},
			SchemaKeyRestURL: {
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
		apiToken := os.Getenv("BUILDKITE_API_TOKEN")
		graphqlUrl := defaultGraphqlEndpoint
		organization := getenv("BUILDKITE_ORGANIZATION_SLUG")
		restUrl := defaultRestEndpoint

		if v, ok := d.Get(SchemaKeyAPIToken).(string); ok && v != "" {
			apiToken = v
		}
		if v, ok := d.Get(SchemaKeyGraphqlURL).(string); ok && v != "" {
			graphqlUrl = v
		} else if v, ok := os.LookupEnv("BUILDKITE_GRAPHQL_URL"); ok {
			graphqlUrl = v
		}
		if v, ok := d.Get(SchemaKeyOrganization).(string); ok && v != "" {
			organization = v
		}
		if v, ok := d.Get(SchemaKeyRestURL).(string); ok && v != "" {
			restUrl = v
		} else if v, ok := os.LookupEnv("BUILDKITE_REST_URL"); ok {
			restUrl = v
		}

		config := &clientConfig{
			org:        organization,
			apiToken:   apiToken,
			graphqlURL: graphqlUrl,
			restURL:    restUrl,
			userAgent:  userAgent,
		}

		return NewClient(config)
	}
}
