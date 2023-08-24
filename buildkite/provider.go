package buildkite

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	defaultGraphqlEndpoint = "https://graphql.buildkite.com/v1"
	defaultRestEndpoint    = "https://api.buildkite.com"

	DefaultTimeout = 30 * time.Second
)

const (
	SchemaKeyOrganization = "organization"
	SchemaKeyAPIToken     = "api_token"
	SchemaKeyGraphqlURL   = "graphql_url"
	SchemaKeyRestURL      = "rest_url"
)

type terraformProvider struct {
	version                 string
	archivePipelineOnDelete bool
}

type providerModel struct {
	ApiToken                types.String   `tfsdk:"api_token"`
	ArchivePipelineOnDelete types.Bool     `tfsdk:"archive_pipeline_on_delete"`
	GraphqlUrl              types.String   `tfsdk:"graphql_url"`
	Organization            types.String   `tfsdk:"organization"`
	RestUrl                 types.String   `tfsdk:"rest_url"`
	Timeouts                timeouts.Value `tfsdk:"timeouts"`
}

func (tf *terraformProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	tf.archivePipelineOnDelete = data.ArchivePipelineOnDelete.ValueBool()

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

	config := clientConfig{
		apiToken:   apiToken,
		graphqlURL: graphqlUrl,
		org:        organization,
		restURL:    restUrl,
		timeouts:   data.Timeouts,
		userAgent:  userAgent("buildkite", tf.version, req.TerraformVersion),
	}
	client, err := NewClient(&config)

	if err != nil {
		resp.Diagnostics.AddError(err.Error(), fmt.Sprintf("... details ... %s", err))
	}

	resp.ResourceData = client
	resp.DataSourceData = client
}

func userAgent(providerName, providerVersion, tfVersion string) string {
	userAgentHeader := fmt.Sprintf("Terraform/%s (+https://www.terraform.io)", tfVersion)
	if providerName != "" {
		userAgentHeader += " " + providerName
		if providerVersion != "" {
			userAgentHeader += "/" + providerVersion
		}
	}

	if addDetails := os.Getenv("TF_APPEND_USER_AGENT"); addDetails != "" {
		addDetails = strings.TrimSpace(addDetails)
		if len(addDetails) > 0 {
			userAgentHeader += " " + addDetails
			log.Printf("[DEBUG] Using modified User-Agent: %s", userAgentHeader)
		}
	}

	return userAgentHeader
}

func (*terraformProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newClusterDatasource,
		newOrganizationDatasource,
		newMetaDatasource,
		newTeamDatasource,
		newPipelineDatasource,
	}
}

func (tf *terraformProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "buildkite"
	resp.Version = tf.version
}

func (tf *terraformProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewClusterQueueResource,
		NewPipelineScheduleResource,
		newAgentTokenResource,
		newClusterAgentTokenResource,
		newClusterResource,
		newOrganizationResource,
		newPipelineResource(tf.archivePipelineOnDelete),
		newTeamMemberResource,
		newTeamResource,
		newTestSuiteResource,
		newPipelineTeamResource,
		newTestSuiteTeamResource,
	}
}

func (*terraformProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			SchemaKeyOrganization: schema.StringAttribute{
				Optional:    true,
				Description: "The Buildkite organization slug",
			},
			SchemaKeyAPIToken: schema.StringAttribute{
				Optional:    true,
				Description: "API token with GraphQL access and `write_pipelines, read_pipelines` and `write_suites` REST API scopes",
				Sensitive:   true,
			},
			SchemaKeyGraphqlURL: schema.StringAttribute{
				Optional:    true,
				Description: "Base URL for the GraphQL API to use",
			},
			SchemaKeyRestURL: schema.StringAttribute{
				Optional:    true,
				Description: "Base URL for the REST API to use",
			},
			"archive_pipeline_on_delete": schema.BoolAttribute{
				Optional:    true,
				Description: "Archive pipelines when destroying instead of completely deleting.",
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.BlockAll(ctx),
		},
	}
}

// New is a helper function to simplify provider server and testing implementation.
func New(version string) provider.Provider {
	return &terraformProvider{
		version: version,
	}
}
