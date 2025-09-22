package buildkite

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
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

	DefaultTimeout               = 180 * time.Second
	DefaultRetryMaxAttempts      = 10
	DefaultRetryWaitMinSeconds   = 15
	DefaultRetryWaitMaxSeconds   = 180
	DefaultGraphQLWaitMaxSeconds = 600
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
	MaxRetries              types.Int64    `tfsdk:"max_retries"`
	Organization            types.String   `tfsdk:"organization"`
	RestURL                 types.String   `tfsdk:"rest_url"`
	Timeouts                timeouts.Value `tfsdk:"timeouts"`
}

func (tf *terraformProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	tf.archivePipelineOnDelete = data.ArchivePipelineOnDelete.ValueBool()

	apiToken := os.Getenv("BUILDKITE_API_TOKEN")
	graphqlUrl := defaultGraphqlEndpoint
	organization := getenv("BUILDKITE_ORGANIZATION_SLUG")
	restURL := defaultRestEndpoint

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
	if data.RestURL.ValueString() != "" {
		restURL = data.RestURL.ValueString()
	} else if v, ok := os.LookupEnv("BUILDKITE_REST_URL"); ok {
		restURL = v
	}

	maxRetries := DefaultRetryMaxAttempts
	if !data.MaxRetries.IsNull() {
		maxRetries = int(data.MaxRetries.ValueInt64())
	}

	config := clientConfig{
		apiToken:   strings.TrimSpace(apiToken),
		graphqlURL: strings.TrimSpace(graphqlUrl),
		org:        strings.TrimSpace(organization),
		restURL:    strings.TrimSpace(restURL),
		timeouts:   data.Timeouts,
		userAgent:  userAgent("buildkite", tf.version, req.TerraformVersion),
		maxRetries: maxRetries,
	}
	client := NewClient(&config)

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
		newClustersDatasource,
		newMetaDatasource,
		newOrganizationDatasource,
		newOrganizationMemberDatasource,
		newOrganizationMembersDatasource,
		newOrganizationRuleDatasource,
		newPipelineDatasource,
		newPipelineTemplateDatasource,
		newRegistryDatasource,
		newSignedPipelineStepsDataSource,
		newTeamDatasource,
		newTeamsDatasource,
		newTestSuiteDatasource,
	}
}

func (tf *terraformProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "buildkite"
	resp.Version = tf.version
}

func (tf *terraformProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newAgentTokenResource,
		newClusterAgentTokenResource,
		newClusterQueueResource,
		newClusterResource,
		newDefaultQueueClusterResource,
		newOrganizationBannerResource,
		newOrganizationRuleResource,
		newOrganizationResource,
		newPipelineScheduleResource,
		newPipelineTeamResource,
		newPipelineTemplateResource,
		newPipelineResource(&tf.archivePipelineOnDelete),
		newRegistryResource,
		newTeamMemberResource,
		newTeamResource,
		newTestSuiteResource,
		newTestSuiteTeamResource,
	}
}

func (*terraformProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			This provider can be used to manage resources on [buildkite.com](https://buildkite.com).
		`),
		Attributes: map[string]schema.Attribute{
			SchemaKeyOrganization: schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The Buildkite organization slug. This can be found on the [settings](https://buildkite.com/organizations/~/settings) page. If not provided, the value is taken from the `BUILDKITE_ORGANIZATION_SLUG` environment variable.",
			},
			SchemaKeyAPIToken: schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "API token with GraphQL access and `write_pipelines`, `read_pipelines` and `write_suites` REST API scopes. You can generate a token from [your settings page](https://buildkite.com/user/api-access-tokens/new?description=terraform&scopes[]=write_pipelines&scopes[]=write_suites&scopes[]=read_pipelines&scopes[]=graphql). If not provided, the value is taken from the `BUILDKITE_API_TOKEN` environment variable.",
				Sensitive:           true,
			},
			SchemaKeyGraphqlURL: schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Base URL for the GraphQL API to use. If not provided, the value is taken from the `BUILDKITE_GRAPHQL_URL` environment variable.",
			},
			SchemaKeyRestURL: schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Base URL for the REST API to use. If not provided, the value is taken from the `BUILDKITE_REST_URL` environment variable.",
			},
			"archive_pipeline_on_delete": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Enable this to archive pipelines when destroying the resource. This is opposed to completely deleting pipelines.",
			},
			"max_retries": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Maximum number of retry attempts for retryable HTTP requests. Defaults to 10.",
			},
			"timeouts": timeouts.AttributesAll(ctx),
		},
	}
}

// New is a helper function to simplify provider server and testing implementation.
func New(version string) provider.Provider {
	return &terraformProvider{
		version: version,
	}
}
