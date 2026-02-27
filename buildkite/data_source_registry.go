package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

// Ensure provider defined types fully satisfy framework interfaces
var (
	_ datasource.DataSource              = &registryDatasource{}
	_ datasource.DataSourceWithConfigure = &registryDatasource{}
)

func newRegistryDatasource() datasource.DataSource {
	return &registryDatasource{}
}

type registryDatasource struct {
	client *Client
}

type registryDatasourceModel struct {
	ID           types.String `tfsdk:"id"`
	UUID         types.String `tfsdk:"uuid"`
	Name         types.String `tfsdk:"name"`
	Slug         types.String `tfsdk:"slug"`
	Ecosystem    types.String `tfsdk:"ecosystem"`
	Description  types.String `tfsdk:"description"`
	Emoji        types.String `tfsdk:"emoji"`
	Color        types.String `tfsdk:"color"`
	OIDCPolicy   types.String `tfsdk:"oidc_policy"`
	Public       types.Bool   `tfsdk:"public"`
	RegistryType types.String `tfsdk:"registry_type"`
	TeamIDs      types.List   `tfsdk:"team_ids"`
}

func (d *registryDatasource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_registry"
}

func (d *registryDatasource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.client = req.ProviderData.(*Client)
}

func (d *registryDatasource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			Use this data source to retrieve information about a Buildkite Package Registry.

			A package registry is a private repository for your organization's packages.
			See https://buildkite.com/docs/packages for more information.
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the registry.",
			},
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the registry.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the registry.",
			},
			"slug": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The slug of the registry. This is used to identify the registry.",
			},
			"ecosystem": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ecosystem of the registry (e.g. `NPM`, `RUBYGEMS`, `DOCKER`).",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A description for the registry.",
			},
			"emoji": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "An emoji to use with the registry.",
			},
			"color": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A color representation of the registry.",
			},
			"oidc_policy": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The registry's OIDC policy.",
			},
			"public": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the registry is publicly accessible.",
			},
			"registry_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The type of the registry (e.g. `source`).",
			},
			"team_ids": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "A list of team GraphQL IDs that have access to this registry.",
			},
		},
	}
}

func (d *registryDatasource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state registryDatasourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...) // Use req.Config for data sources
	if resp.Diagnostics.HasError() {
		return
	}

	timeoutDuration, diags := d.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var dataFound bool

	err := retry.RetryContext(ctx, timeoutDuration, func() *retry.RetryError {
		rawSlug := state.Slug.ValueString()
		var fullSlug string
		if strings.Contains(rawSlug, "/") {
			fullSlug = rawSlug
		} else {
			fullSlug = fmt.Sprintf("%s/%s", d.client.organization, rawSlug)
		}

		apiPathSlug := rawSlug
		if i := strings.LastIndexByte(rawSlug, '/'); i != -1 {
			apiPathSlug = rawSlug[i+1:]
		}

		url := fmt.Sprintf("%s/v2/packages/organizations/%s/registries/%s", d.client.restURL, d.client.organization, apiPathSlug)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error creating HTTP request: %w", err))
		}
		req.Header.Set("Accept", "application/json")

		httpResp, err := d.client.http.Do(req)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error making HTTP request to %s: %w", url, err))
		}
		defer httpResp.Body.Close()

		if httpResp.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddWarning("Registry not found", fmt.Sprintf("No registry found with slug '%s' (resolved to '%s') at %s", state.Slug.ValueString(), fullSlug, url))
			resp.State.RemoveResource(ctx)
			dataFound = false
			return nil
		}

		if httpResp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(httpResp.Body)
			return retry.NonRetryableError(fmt.Errorf("error fetching registry (status %d from %s): %s", httpResp.StatusCode, url, string(bodyBytes)))
		}

		var result registryAPIResponse
		if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
			return retry.NonRetryableError(fmt.Errorf("error decoding registry response body from %s: %w", url, err))
		}

		if result.GraphQLID == "" {
			resp.Diagnostics.AddWarning("Registry data incomplete", fmt.Sprintf("Registry found with slug '%s' but essential data (GraphqlID) is missing from response at %s", fullSlug, url))
			resp.State.RemoveResource(ctx)
			dataFound = false
			return nil
		}

		dataFound = true
		state.ID = types.StringValue(result.GraphQLID)
		state.UUID = types.StringValue(result.ID)
		state.Name = types.StringValue(result.Name)
		state.Slug = types.StringValue(result.Slug)
		state.Ecosystem = types.StringValue(result.Ecosystem)

		state.Description = optionalStringValue(result.Description)
		state.Emoji = optionalStringValue(result.Emoji)
		state.Color = optionalStringValue(result.Color)
		state.OIDCPolicy = optionalStringValue(result.OIDCPolicy)

		state.Public = types.BoolValue(result.Public)
		state.RegistryType = types.StringValue(result.RegistryType)
		state.TeamIDs = handleTeamIDs(result.TeamIDs, state.TeamIDs)

		return nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to query Registry", fmt.Sprintf("Failed to query Registry with slug '%s' after multiple attempts: %s", state.Slug.ValueString(), err.Error()))
		return
	}

	if dataFound {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}
