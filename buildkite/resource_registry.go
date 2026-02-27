package buildkite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	bkplanmodifier "github.com/buildkite/terraform-provider-buildkite/internal/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type registryResource struct {
	client *Client
}

type registryResourceModel struct {
	ID           types.String `tfsdk:"id"`
	UUID         types.String `tfsdk:"uuid"`
	Name         types.String `tfsdk:"name"`
	Ecosystem    types.String `tfsdk:"ecosystem"`
	Description  types.String `tfsdk:"description"`
	Emoji        types.String `tfsdk:"emoji"`
	Color        types.String `tfsdk:"color"`
	OIDCPolicy   types.String `tfsdk:"oidc_policy"`
	Public       types.Bool   `tfsdk:"public"`
	RegistryType types.String `tfsdk:"registry_type"`
	Slug         types.String `tfsdk:"slug"`
	TeamIDs      types.List   `tfsdk:"team_ids"`
}

func newRegistryResource() resource.Resource {
	return &registryResource{}
}

func (p *registryResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_registry"
}

func (p *registryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	p.client = req.ProviderData.(*Client)
}

func (p *registryResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resource_schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			This resource allows you to create and manage a Buildkite Registry.
			Find out more information in our [documentation](https://buildkite.com/docs/package-registries).
		`),
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the registry.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the registry.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"slug": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The slug of the registry.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					bkplanmodifier.UseStateIfUnchanged("name"),
				},
			},
			"name": resource_schema.StringAttribute{
				MarkdownDescription: "The name of the registry. Can only contain numbers and letters, no spaces or special characters.",
				Required:            true,
			},
			"ecosystem": resource_schema.StringAttribute{
				MarkdownDescription: "The ecosystem of the registry. **Warning:** This value cannot be changed after creation. Any attempts to update this field will result in API errors.",
				Required:            true,
			},
			"description": resource_schema.StringAttribute{
				Optional: true,
				MarkdownDescription: heredoc.Doc(`
					This is a description for the registry, this may describe the usage for it, the region, or something else
					which would help identify the registry's purpose.
				`),
			},
			"emoji": resource_schema.StringAttribute{
				Optional: true,
				MarkdownDescription: heredoc.Doc(`
					An emoji to use with the registry, this can either be set using :buildkite: notation, or with the
					emoji itself, such as 🚀.
				`),
			},
			"color": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A color representation of the registry. Accepts hex codes, eg #BADA55.",
			},
			"oidc_policy": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The registry's OIDC policy.",
			},
			"public": resource_schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the registry is publicly accessible.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"registry_type": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The type of the registry (e.g. `source`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"team_ids": resource_schema.ListAttribute{
				Optional:            true,
				MarkdownDescription: "The team IDs that have access to the registry.",
				ElementType:         types.StringType,
			},
		},
	}
}

func (p *registryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state *registryResourceModel

	diags := req.Plan.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	timeout, diags := p.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		url := fmt.Sprintf("%s/v2/packages/organizations/%s/registries", p.client.restURL, p.client.organization)

		reqBody := map[string]interface{}{
			"name":      state.Name.ValueString(),
			"ecosystem": state.Ecosystem.ValueString(),
		}

		// Add optional fields if they're set
		if !state.Description.IsNull() && !state.Description.IsUnknown() {
			reqBody["description"] = state.Description.ValueString()
		}
		if !state.Emoji.IsNull() && !state.Emoji.IsUnknown() {
			reqBody["emoji"] = state.Emoji.ValueString()
		}
		if !state.Color.IsNull() && !state.Color.IsUnknown() {
			reqBody["color"] = state.Color.ValueString()
		}
		if !state.OIDCPolicy.IsNull() && !state.OIDCPolicy.IsUnknown() {
			reqBody["oidc_policy"] = state.OIDCPolicy.ValueString()
		}

		// Get team IDs from the state
		if !state.TeamIDs.IsNull() && !state.TeamIDs.IsUnknown() {
			teamIDs := make([]string, 0)
			teamIDsElements := state.TeamIDs.Elements()

			for _, element := range teamIDsElements {
				if strVal, ok := element.(types.String); ok {
					teamIDs = append(teamIDs, strVal.ValueString())
				}
			}

			if len(teamIDs) > 0 {
				reqBody["team_ids"] = teamIDs
			}
		}

		// Marshal the request body to JSON
		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error marshaling request body: %w", err))
		}

		// Create the HTTP request
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error creating HTTP request: %w", err))
		}

		// Set request headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		// Execute the HTTP request
		resp, err := p.client.http.Do(req)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error making HTTP request: %w", err))
		}
		defer resp.Body.Close()

		// Check for successful response
		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return retry.NonRetryableError(fmt.Errorf("error creating registry (status %d): %s", resp.StatusCode, string(bodyBytes)))
		}

		// Parse the response
		var result registryAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return retry.NonRetryableError(fmt.Errorf("error decoding response body: %w", err))
		}

		if result.ID == "" {
			return retry.NonRetryableError(fmt.Errorf("API response missing required ID field"))
		}

		mapRegistryResponseToModel(&result, state)

		return nil
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating registry",
			fmt.Sprintf("Could not create registry: %s", err),
		)
		return
	}

	// Make sure to save state properly including the UUID
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Implementation for Read function with team_ids handling
func (p *registryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var registryNotFound bool
	var state *registryResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := p.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if state.Slug.IsNull() || state.Slug.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Error reading registry",
			"Registry slug is not set in state. Please re-import the resource using: terraform import <address> <slug>",
		)
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		url := fmt.Sprintf("%s/v2/packages/organizations/%s/registries/%s", p.client.restURL, p.client.organization, state.Slug.ValueString())

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error creating HTTP request: %w", err))
		}

		req.Header.Set("Accept", "application/json")

		resp, err := p.client.http.Do(req)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error making HTTP request: %w", err))
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			registryNotFound = true
			return nil
		}

		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return retry.NonRetryableError(fmt.Errorf("error reading registry (status %d): %s", resp.StatusCode, string(bodyBytes)))
		}

		var result registryAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return retry.NonRetryableError(fmt.Errorf("error decoding response body: %w", err))
		}

		mapRegistryResponseToModel(&result, state)

		return nil
	})

	if registryNotFound {
		resp.Diagnostics.AddWarning(
			"Registry not found",
			fmt.Sprintf("Registry %s was not found, removing from state", state.Slug.ValueString()),
		)
		resp.State.RemoveResource(ctx)
		return
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading registry",
			fmt.Sprintf("Could not read registry: %s", err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Implementation for Update function with team_ids handling
func (p *registryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state *registryResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := p.client.timeouts.Update(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if state.Slug.IsNull() || state.Slug.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Error updating registry",
			"Registry slug is not set in state. Please re-import the resource using: terraform import <address> <slug>",
		)
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		url := fmt.Sprintf("%s/v2/packages/organizations/%s/registries/%s", p.client.restURL, p.client.organization, state.Slug.ValueString())

		reqBody := map[string]interface{}{
			"name": plan.Name.ValueString(),
		}

		if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
			reqBody["description"] = plan.Description.ValueString()
		} else if !state.Description.IsNull() {
			reqBody["description"] = nil
		}
		if !plan.Emoji.IsNull() && !plan.Emoji.IsUnknown() {
			reqBody["emoji"] = plan.Emoji.ValueString()
		} else if !state.Emoji.IsNull() {
			reqBody["emoji"] = nil
		}
		if !plan.Color.IsNull() && !plan.Color.IsUnknown() {
			reqBody["color"] = plan.Color.ValueString()
		} else if !state.Color.IsNull() {
			reqBody["color"] = nil
		}
		if !plan.OIDCPolicy.IsNull() && !plan.OIDCPolicy.IsUnknown() {
			reqBody["oidc_policy"] = plan.OIDCPolicy.ValueString()
		} else if !state.OIDCPolicy.IsNull() {
			reqBody["oidc_policy"] = nil
		}

		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error marshaling request body: %w", err))
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewBuffer(jsonBody))
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error creating HTTP request: %w", err))
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := p.client.http.Do(req)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error making HTTP request: %w", err))
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return retry.NonRetryableError(fmt.Errorf("registry %s not found, it may have been deleted outside of Terraform", state.Slug.ValueString()))
		} else if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return retry.NonRetryableError(fmt.Errorf("error updating registry (status %d): %s", resp.StatusCode, string(bodyBytes)))
		}

		var result registryAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return retry.NonRetryableError(fmt.Errorf("error decoding response body: %w", err))
		}

		mapRegistryResponseToModel(&result, plan)

		return nil
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating registry",
			fmt.Sprintf("Could not update registry: %s", err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// registryAPIResponse represents the JSON shape returned by the Packages REST API
// for registry endpoints (GET, POST, PUT).
type registryAPIResponse struct {
	GraphQLID    string   `json:"graphql_id"`
	ID           string   `json:"id"`
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Ecosystem    string   `json:"ecosystem"`
	Description  *string  `json:"description"`
	Emoji        *string  `json:"emoji"`
	Color        *string  `json:"color"`
	OIDCPolicy   *string  `json:"oidc_policy"`
	Public       bool     `json:"public"`
	RegistryType string   `json:"type"`
	TeamIDs      []string `json:"team_ids"`
}

// mapRegistryResponseToModel maps an API response onto a Terraform resource model.
func mapRegistryResponseToModel(result *registryAPIResponse, model *registryResourceModel) {
	model.ID = types.StringValue(result.GraphQLID)
	model.UUID = types.StringValue(result.ID)
	model.Slug = types.StringValue(result.Slug)
	model.Name = types.StringValue(result.Name)
	model.Ecosystem = types.StringValue(result.Ecosystem)
	model.Description = optionalStringValue(result.Description)
	model.Emoji = optionalStringValue(result.Emoji)
	model.Color = optionalStringValue(result.Color)
	model.OIDCPolicy = optionalStringValue(result.OIDCPolicy)
	model.Public = types.BoolValue(result.Public)
	model.RegistryType = types.StringValue(result.RegistryType)
	model.TeamIDs = handleTeamIDs(result.TeamIDs, model.TeamIDs)
}

// Ensures consistent handling of team_ids field
func handleTeamIDs(apiTeamIDs []string, existing types.List) types.List {
	if len(apiTeamIDs) > 0 {
		// API returned team IDs, set them in the state
		teamIDElements := make([]attr.Value, 0, len(apiTeamIDs))
		for _, id := range apiTeamIDs {
			teamIDElements = append(teamIDElements, types.StringValue(id))
		}

		teamIDsList, diags := types.ListValue(types.StringType, teamIDElements)
		if diags.HasError() {
			// If there's an error, preserve the original value
			return existing
		}
		return teamIDsList
	} else if !existing.IsNull() && len(existing.Elements()) > 0 {
		return existing
	} else {
		// No team IDs present, set to null
		return types.ListNull(types.StringType)
	}
}

// optionalStringValue maps a nullable API response field to the appropriate Terraform type.
// nil (JSON null or omitted) becomes types.StringNull(); non-nil becomes types.StringValue.
func optionalStringValue(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

func (p *registryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *registryResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := p.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if state.Slug.IsNull() || state.Slug.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Error deleting registry",
			"Registry slug is not set in state. Please re-import the resource using: terraform import <address> <slug>",
		)
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		url := fmt.Sprintf("%s/v2/packages/organizations/%s/registries/%s", p.client.restURL, p.client.organization, state.Slug.ValueString())

		// Create the HTTP request
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error creating HTTP request: %w", err))
		}

		// Execute the HTTP request
		resp, err := p.client.http.Do(req)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error making HTTP request: %w", err))
		}
		defer resp.Body.Close()

		// If the registry was already deleted, consider the delete successful
		if resp.StatusCode == http.StatusNotFound {
			return nil
		}

		// Check for successful response
		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return retry.NonRetryableError(fmt.Errorf("error deleting registry (status %d): %s", resp.StatusCode, string(bodyBytes)))
		}

		return nil
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting registry",
			fmt.Sprintf("Could not delete registry: %s", err),
		)
		return
	}

	// Resource successfully deleted
	resp.State.RemoveResource(ctx)
}

// ModifyPlan is called by the Terraform Framework during planning
func (r *registryResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// If there is no plan, we have nothing to do
	if req.Plan.Raw.IsNull() {
		return
	}

	// Get the current plan
	var plan registryResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the current state
	if req.State.Raw.IsNull() {
		// No state means this is a create, nothing to do
		return
	}

	var state registryResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check if name has changed
	if !plan.Name.Equal(state.Name) {
		// If name changed, mark slug as unknown since it will be regenerated by the API
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("slug"), types.StringUnknown())...)
	}

	// Reject changes to immutable fields at plan time
	if !plan.Ecosystem.Equal(state.Ecosystem) {
		resp.Diagnostics.AddError(
			"Ecosystem change detected",
			"The ecosystem attribute cannot be changed after registry creation. This restriction is enforced by the Buildkite API.",
		)
	}

	if !plan.TeamIDs.Equal(state.TeamIDs) {
		resp.Diagnostics.AddError(
			"Team IDs change detected",
			"The team_ids attribute cannot be changed after registry creation. This restriction is enforced by the Buildkite API.",
		)
	}
}

func (r *registryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("slug"), req, resp)

	// After import, we'll need to do a read to get the proper data into the state
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringNull())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("uuid"), types.StringNull())...)
}
