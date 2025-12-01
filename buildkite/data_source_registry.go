package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	ID          types.String `tfsdk:"id"`          // GraphQL ID
	UUID        types.String `tfsdk:"uuid"`        // UUID
	Name        types.String `tfsdk:"name"`        // Name
	Slug        types.String `tfsdk:"slug"`        // Slug (used for lookup)
	Ecosystem   types.String `tfsdk:"ecosystem"`   // PackageEcosystem
	Description types.String `tfsdk:"description"` // Description
	Emoji       types.String `tfsdk:"emoji"`       // Emoji
	Color       types.String `tfsdk:"color"`       // Color
	OIDCPolicy  types.String `tfsdk:"oidc_policy"` // OIDC Policy
	TeamIDs     types.List   `tfsdk:"team_ids"`    // List of Team GraphQL IDs
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
			return retry.RetryableError(fmt.Errorf("error making HTTP request to %s: %w", url, err))
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
			return retry.RetryableError(fmt.Errorf("error fetching registry (status %d from %s): %s", httpResp.StatusCode, url, string(bodyBytes)))
		}

		var result struct {
			GraphqlID   string   `json:"graphql_id"`
			ID          string   `json:"id"` // This is the UUID
			Slug        string   `json:"slug"`
			Name        string   `json:"name"`
			Ecosystem   string   `json:"ecosystem"`
			Description string   `json:"description"`
			Emoji       string   `json:"emoji"`
			Color       string   `json:"color"`
			OIDCPolicy  string   `json:"oidc_policy"`
			TeamIDs     []string `json:"team_ids"`
		}

		if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
			return retry.NonRetryableError(fmt.Errorf("error decoding registry response body from %s: %w", url, err))
		}

		if result.GraphqlID == "" { // Check if essential data is missing
			resp.Diagnostics.AddWarning("Registry data incomplete", fmt.Sprintf("Registry found with slug '%s' but essential data (GraphqlID) is missing from response at %s", fullSlug, url))
			resp.State.RemoveResource(ctx)
			dataFound = false
			return nil
		}

		dataFound = true
		state.ID = types.StringValue(result.GraphqlID)
		state.UUID = types.StringValue(result.ID)
		state.Name = types.StringValue(result.Name)
		state.Slug = types.StringValue(result.Slug) // Re-affirm from response (this should be the simple slug)
		state.Ecosystem = types.StringValue(result.Ecosystem)

		if result.Description != "" || !state.Description.IsNull() { // Update if API provides it or clear if API clears it and it was set
			state.Description = types.StringValue(result.Description)
		}
		if result.Emoji != "" || !state.Emoji.IsNull() {
			state.Emoji = types.StringValue(result.Emoji)
		}
		if result.Color != "" || !state.Color.IsNull() {
			state.Color = types.StringValue(result.Color)
		}
		if result.OIDCPolicy != "" || !state.OIDCPolicy.IsNull() {
			state.OIDCPolicy = types.StringValue(result.OIDCPolicy)
		}

		// Handle team_ids using logic similar to handleTeamIDs from resource_registry.go
		if len(result.TeamIDs) > 0 {
			teams := make([]attr.Value, len(result.TeamIDs))
			for i, id := range result.TeamIDs {
				teams[i] = types.StringValue(id)
			}
			state.TeamIDs = types.ListValueMust(types.StringType, teams)
		} else {
			state.TeamIDs = types.ListNull(types.StringType) // If API returns empty, set to null
		}

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
