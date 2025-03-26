package buildkite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type registryResource struct {
	client *Client
}

type registryResourceModel struct {
	ID          types.String `tfsdk:"id"`
	UUID        types.String `tfsdk:"uuid"`
	Name        types.String `tfsdk:"name"`
	Ecosystem   types.String `tfsdk:"ecosystem"`
	Description types.String `tfsdk:"description"`
	Emoji       types.String `tfsdk:"emoji"`
	Color       types.String `tfsdk:"color"`
	OIDCPolicy  types.String `tfsdk:"oidc_policy"`
	Slug        types.String `tfsdk:"slug"`
	TeamIDs     types.List   `tfsdk:"team_ids"`
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
			return retry.RetryableError(fmt.Errorf("error making HTTP request: %w", err))
		}
		defer resp.Body.Close()

		// Check for successful response
		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return retry.RetryableError(fmt.Errorf("error creating registry (status %d): %s", resp.StatusCode, string(bodyBytes)))
		}

		// Parse the response
		var result struct {
			GraphqlID   string   `json:"graphql_id"`
			ID          string   `json:"id"`
			Slug        string   `json:"slug"`
			Name        string   `json:"name"`
			Ecosystem   string   `json:"ecosystem"`
			Description string   `json:"description"`
			Emoji       string   `json:"emoji"`
			Color       string   `json:"color"`
			OIDCPolicy  string   `json:"oidc_policy"`
			TeamIDs     []string `json:"team_ids"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return retry.NonRetryableError(fmt.Errorf("error decoding response body: %w", err))
		}

		// Ensure we have ID set in state - this is the UUID we need
		if result.ID == "" {
			return retry.NonRetryableError(fmt.Errorf("API response missing required ID field"))
		}

		// Set the GraphQL ID as the Terraform ID and the API ID as the UUID
		state.ID = types.StringValue(result.GraphqlID)
		state.UUID = types.StringValue(result.ID)
		state.Slug = types.StringValue(result.Slug)
		state.Name = types.StringValue(result.Name)
		state.Ecosystem = types.StringValue(result.Ecosystem)

		if result.Description != "" {
			state.Description = types.StringValue(result.Description)
		}
		if result.Emoji != "" {
			state.Emoji = types.StringValue(result.Emoji)
		}
		if result.Color != "" {
			state.Color = types.StringValue(result.Color)
		}
		if result.OIDCPolicy != "" {
			state.OIDCPolicy = types.StringValue(result.OIDCPolicy)
		}

		// Handle the team_ids response using the helper function
		state.TeamIDs = handleTeamIDs(result.TeamIDs, state.TeamIDs)

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

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		// Check if we have a UUID to work with
		if state.Slug.IsNull() || state.Slug.ValueString() == "" {
			// If no slug is found, we need to fetch all registries and find by name
			// This handles the case during import or when slug isn't in state
			url := fmt.Sprintf("%s/v2/packages/organizations/%s/registries", p.client.restURL, p.client.organization)

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return retry.NonRetryableError(fmt.Errorf("error creating HTTP request: %w", err))
			}

			req.Header.Set("Accept", "application/json")

			resp, err := p.client.http.Do(req)
			if err != nil {
				return retry.RetryableError(fmt.Errorf("error making HTTP request: %w", err))
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 400 {
				bodyBytes, _ := io.ReadAll(resp.Body)
				return retry.RetryableError(fmt.Errorf("error listing registries (status %d): %s", resp.StatusCode, string(bodyBytes)))
			}

			// Read and parse all registries
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return retry.NonRetryableError(fmt.Errorf("error reading response body: %w", err))
			}

			var registries []struct {
				GraphqlID   string   `json:"graphql_id"`
				ID          string   `json:"id"`
				Slug        string   `json:"slug"`
				Name        string   `json:"name"`
				Ecosystem   string   `json:"ecosystem"`
				Description string   `json:"description,omitempty"`
				Emoji       string   `json:"emoji,omitempty"`
				Color       string   `json:"color,omitempty"`
				OIDCPolicy  string   `json:"oidc_policy,omitempty"`
				TeamIDs     []string `json:"team_ids,omitempty"`
			}

			if err := json.Unmarshal(bodyBytes, &registries); err != nil {
				return retry.NonRetryableError(fmt.Errorf("error decoding response body: %w", err))
			}

			// Find the registry by name
			found := false
			for _, registry := range registries {
				if registry.Name == state.Name.ValueString() {
					// Found a match, update state with complete information including UUID
					state.ID = types.StringValue(registry.GraphqlID)
					state.UUID = types.StringValue(registry.ID)
					state.Slug = types.StringValue(registry.Slug)
					state.Name = types.StringValue(registry.Name)
					state.Ecosystem = types.StringValue(registry.Ecosystem)

					if registry.Description != "" {
						state.Description = types.StringValue(registry.Description)
					} else {
						state.Description = types.StringNull()
					}

					if registry.Emoji != "" {
						state.Emoji = types.StringValue(registry.Emoji)
					} else {
						state.Emoji = types.StringNull()
					}

					if registry.Color != "" {
						state.Color = types.StringValue(registry.Color)
					} else {
						state.Color = types.StringNull()
					}

					if registry.OIDCPolicy != "" {
						state.OIDCPolicy = types.StringValue(registry.OIDCPolicy)
					} else {
						state.OIDCPolicy = types.StringNull()
					}

					// Handle the team_ids response
					state.TeamIDs = handleTeamIDs(registry.TeamIDs, state.TeamIDs)

					found = true
					break
				}
			}

			if !found {
				registryNotFound = true
				return nil
			}

			return nil
		}

		// We have a UUID, use it to directly fetch the registry
		url := fmt.Sprintf("%s/v2/packages/organizations/%s/registries/%s", p.client.restURL, p.client.organization, state.Slug.ValueString())

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error creating HTTP request: %w", err))
		}

		req.Header.Set("Accept", "application/json")

		resp, err := p.client.http.Do(req)
		if err != nil {
			return retry.RetryableError(fmt.Errorf("error making HTTP request: %w", err))
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			registryNotFound = true
			return nil
		}

		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return retry.RetryableError(fmt.Errorf("error reading registry (status %d): %s", resp.StatusCode, string(bodyBytes)))
		}

		// Read the entire response body to check if it's an array or a single object
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error reading response body: %w", err))
		}

		// Define the struct for a single registry
		var result struct {
			GraphQLID   string   `json:"graphql_id"`
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Slug        string   `json:"slug"`
			Ecosystem   string   `json:"ecosystem"`
			Description string   `json:"description,omitempty"`
			Emoji       string   `json:"emoji,omitempty"`
			Color       string   `json:"color,omitempty"`
			OIDCPolicy  string   `json:"oidc_policy,omitempty"`
			TeamIDs     []string `json:"team_ids,omitempty"`
		}

		// Try to detect if response is an array and handle appropriately
		if len(bodyBytes) > 0 && bodyBytes[0] == '[' {
			// It's an array, so we need to decode as array and find the right registry
			var registries []struct {
				GraphQLID   string   `json:"graphql_id"`
				ID          string   `json:"id"`
				Name        string   `json:"name"`
				Slug        string   `json:"slug"`
				Ecosystem   string   `json:"ecosystem"`
				Description string   `json:"description,omitempty"`
				Emoji       string   `json:"emoji,omitempty"`
				Color       string   `json:"color,omitempty"`
				OIDCPolicy  string   `json:"oidc_policy,omitempty"`
				TeamIDs     []string `json:"team_ids,omitempty"`
			}

			if err := json.Unmarshal(bodyBytes, &registries); err != nil {
				return retry.NonRetryableError(fmt.Errorf("error decoding response body: %w", err))
			}

			// Find the registry with the matching UUID
			found := false
			for _, registry := range registries {
				if registry.ID == state.UUID.ValueString() {
					result = registry
					found = true
					break
				}
			}

			if !found {
				registryNotFound = true
				return nil
			}
		} else {
			// It's a single object (expected format), decode directly
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				return retry.NonRetryableError(fmt.Errorf("error decoding response body: %w", err))
			}
		}

		// Update the state with the found registry data
		state.ID = types.StringValue(result.GraphQLID)
		state.UUID = types.StringValue(result.ID)
		state.Slug = types.StringValue(result.Slug)
		state.Name = types.StringValue(result.Name)
		state.Ecosystem = types.StringValue(result.Ecosystem)

		if result.Description != "" {
			state.Description = types.StringValue(result.Description)
		} else {
			state.Description = types.StringNull()
		}

		if result.Emoji != "" {
			state.Emoji = types.StringValue(result.Emoji)
		} else {
			state.Emoji = types.StringNull()
		}

		if result.Color != "" {
			state.Color = types.StringValue(result.Color)
		} else {
			state.Color = types.StringNull()
		}

		if result.OIDCPolicy != "" {
			state.OIDCPolicy = types.StringValue(result.OIDCPolicy)
		} else {
			state.OIDCPolicy = types.StringNull()
		}

		// Handle the team_ids response using the helper function
		state.TeamIDs = handleTeamIDs(result.TeamIDs, state.TeamIDs)

		return nil
	})

	if registryNotFound {
		var idForWarning string
		// Always prefer to show ID in messages for consistency with other resources
		if state.Slug.IsNull() {
			idForWarning = state.ID.ValueString()
		} else {
			idForWarning = state.Slug.ValueString()
		}

		resp.Diagnostics.AddWarning(
			"Registry not found",
			fmt.Sprintf("Registry %s was not found, removing from state", idForWarning),
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

	// Check if ecosystem is being changed and add an error
	if !plan.Ecosystem.Equal(state.Ecosystem) {
		resp.Diagnostics.AddError(
			"Ecosystem change detected",
			"The ecosystem attribute cannot be changed after registry creation. This restriction is enforced by the Buildkite API.",
		)
		return
	}

	timeout, diags := p.client.timeouts.Update(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// First, ensure we have UUID set in state
	if state.UUID.IsNull() || state.UUID.ValueString() == "" {
		// We need to find the registry by name to get its UUID first
		// This helps ensure proper updates when the UUID wasn't saved in the previous run
		readTimeout, _ := p.client.timeouts.Read(ctx, DefaultTimeout)

		err := retry.RetryContext(ctx, readTimeout, func() *retry.RetryError {
			url := fmt.Sprintf("%s/v2/packages/organizations/%s/registries", p.client.restURL, p.client.organization)

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return retry.NonRetryableError(fmt.Errorf("error creating HTTP request: %w", err))
			}

			req.Header.Set("Accept", "application/json")

			resp, err := p.client.http.Do(req)
			if err != nil {
				return retry.RetryableError(fmt.Errorf("error making HTTP request: %w", err))
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 400 {
				bodyBytes, _ := io.ReadAll(resp.Body)
				return retry.RetryableError(fmt.Errorf("error listing registries (status %d): %s", resp.StatusCode, string(bodyBytes)))
			}

			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return retry.NonRetryableError(fmt.Errorf("error reading response body: %w", err))
			}

			var registries []struct {
				GraphqlID string `json:"graphql_id"`
				ID        string `json:"id"`
				Slug      string `json:"slug"`
				Name      string `json:"name"`
				Ecosystem string `json:"ecosystem"`
			}

			if err := json.Unmarshal(bodyBytes, &registries); err != nil {
				return retry.NonRetryableError(fmt.Errorf("error decoding response body: %w", err))
			}

			// Find registry by name
			for _, registry := range registries {
				if registry.Name == state.Name.ValueString() {
					// Update the state with the UUID
					state.UUID = types.StringValue(registry.ID)
					return nil
				}
			}

			// If we didn't find the registry, it might not exist
			return retry.NonRetryableError(fmt.Errorf("registry %s not found", state.Name.ValueString()))
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Error retrieving registry UUID",
				fmt.Sprintf("Could not find registry UUID: %s", err),
			)
			return
		}
	}

	// Now proceed with the update using the UUID we have
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		// First try to use UUID for lookup if it's available
		var lookupID string

		if !state.Slug.IsNull() && state.Slug.ValueString() != "" {
			lookupID = state.Slug.ValueString()
		} else {
			return retry.NonRetryableError(fmt.Errorf("no valid ID or UUID available for registry lookup"))
		}

		url := fmt.Sprintf("%s/v2/packages/organizations/%s/registries/%s", p.client.restURL, p.client.organization, lookupID)

		reqBody := map[string]interface{}{
			"name":      plan.Name.ValueString(),
			"ecosystem": plan.Ecosystem.ValueString(),
		}

		if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
			reqBody["description"] = plan.Description.ValueString()
		}
		if !plan.Emoji.IsNull() && !plan.Emoji.IsUnknown() {
			reqBody["emoji"] = plan.Emoji.ValueString()
		}
		if !plan.Color.IsNull() && !plan.Color.IsUnknown() {
			reqBody["color"] = plan.Color.ValueString()
		}
		if !plan.OIDCPolicy.IsNull() && !plan.OIDCPolicy.IsUnknown() {
			reqBody["oidc_policy"] = plan.OIDCPolicy.ValueString()
		}

		if !plan.Ecosystem.IsNull() && !plan.Ecosystem.IsUnknown() {
			reqBody["ecosystem"] = plan.Ecosystem.ValueString()
		}

		// Handle team_ids in the plan
		if !plan.TeamIDs.IsNull() && !plan.TeamIDs.IsUnknown() {
			teamIDs := make([]string, 0)
			teamIDsElements := plan.TeamIDs.Elements()

			for _, element := range teamIDsElements {
				if strVal, ok := element.(types.String); ok {
					teamIDs = append(teamIDs, strVal.ValueString())
				}
			}

			if len(teamIDs) > 0 {
				reqBody["team_ids"] = teamIDs
			}
		} else {
			// If the plan has an empty list, we should pass an empty array to clear the team IDs
			reqBody["team_ids"] = []string{}
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
			return retry.RetryableError(fmt.Errorf("error making HTTP request: %w", err))
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			// If registry not found during update, check if it exists at all by reading
			readURL := fmt.Sprintf("%s/v2/packages/organizations/%s/registries", p.client.restURL, p.client.organization)
			readReq, err := http.NewRequestWithContext(ctx, http.MethodGet, readURL, nil)
			if err != nil {
				return retry.NonRetryableError(fmt.Errorf("error creating HTTP request: %w", err))
			}

			readReq.Header.Set("Accept", "application/json")
			readResp, err := p.client.http.Do(readReq)
			if err != nil {
				return retry.RetryableError(fmt.Errorf("error making HTTP request: %w", err))
			}
			defer readResp.Body.Close()

			// If we can read the registries, try to recreate instead of update
			if readResp.StatusCode < 400 {
				// Try to create the registry anew since the UUID doesn't seem to exist
				createURL := fmt.Sprintf("%s/v2/packages/organizations/%s/registries", p.client.restURL, p.client.organization)
				createReq, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewBuffer(jsonBody))
				if err != nil {
					return retry.NonRetryableError(fmt.Errorf("error creating HTTP request: %w", err))
				}

				createReq.Header.Set("Content-Type", "application/json")
				createReq.Header.Set("Accept", "application/json")

				createResp, err := p.client.http.Do(createReq)
				if err != nil {
					return retry.RetryableError(fmt.Errorf("error making HTTP request: %w", err))
				}
				defer createResp.Body.Close()

				if createResp.StatusCode >= 400 {
					bodyBytes, _ := io.ReadAll(createResp.Body)
					return retry.RetryableError(fmt.Errorf("error recreating registry (status %d): %s", createResp.StatusCode, string(bodyBytes)))
				}

				// Continue with the response processing using the create response
				resp = createResp
			} else {
				// Something else is wrong with the API
				bodyBytes, _ := io.ReadAll(readResp.Body)
				return retry.RetryableError(fmt.Errorf("error listing registries (status %d): %s", readResp.StatusCode, string(bodyBytes)))
			}
		} else if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return retry.RetryableError(fmt.Errorf("error updating registry (status %d): %s", resp.StatusCode, string(bodyBytes)))
		}

		var result struct {
			GraphQLID   string   `json:"graphql_id"`
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Slug        string   `json:"slug"`
			Ecosystem   string   `json:"ecosystem"`
			Description string   `json:"description,omitempty"`
			Emoji       string   `json:"emoji,omitempty"`
			Color       string   `json:"color,omitempty"`
			OIDCPolicy  string   `json:"oidc_policy,omitempty"`
			TeamIDs     []string `json:"team_ids,omitempty"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return retry.NonRetryableError(fmt.Errorf("error decoding response body: %w", err))
		}

		// Use GraphQL ID for the Terraform resource ID, UUID is for API lookups
		plan.ID = types.StringValue(result.GraphQLID)
		plan.UUID = types.StringValue(result.ID)
		plan.Slug = types.StringValue(result.Slug)
		plan.Name = types.StringValue(result.Name)
		plan.Ecosystem = types.StringValue(result.Ecosystem)

		if result.Description != "" {
			plan.Description = types.StringValue(result.Description)
		} else {
			plan.Description = types.StringNull()
		}

		if result.Emoji != "" {
			plan.Emoji = types.StringValue(result.Emoji)
		} else {
			plan.Emoji = types.StringNull()
		}

		if result.Color != "" {
			plan.Color = types.StringValue(result.Color)
		} else {
			plan.Color = types.StringNull()
		}

		if result.OIDCPolicy != "" {
			plan.OIDCPolicy = types.StringValue(result.OIDCPolicy)
		} else {
			plan.OIDCPolicy = types.StringNull()
		}

		// Handle team_ids in the response using the helper function
		plan.TeamIDs = handleTeamIDs(result.TeamIDs, plan.TeamIDs)

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

	// First, ensure we have UUID set in state if possible
	if state.Slug.IsNull() || state.Slug.ValueString() == "" {
		// We need to find the registry by name to get its slug first
		readTimeout, _ := p.client.timeouts.Read(ctx, DefaultTimeout)

		_ = retry.RetryContext(ctx, readTimeout, func() *retry.RetryError {
			url := fmt.Sprintf("%s/v2/packages/organizations/%s/registries", p.client.restURL, p.client.organization)

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return retry.NonRetryableError(fmt.Errorf("error creating HTTP request: %w", err))
			}

			req.Header.Set("Accept", "application/json")

			resp, err := p.client.http.Do(req)
			if err != nil {
				return retry.RetryableError(fmt.Errorf("error making HTTP request: %w", err))
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 400 {
				bodyBytes, _ := io.ReadAll(resp.Body)
				return retry.RetryableError(fmt.Errorf("error listing registries (status %d): %s", resp.StatusCode, string(bodyBytes)))
			}

			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return retry.NonRetryableError(fmt.Errorf("error reading response body: %w", err))
			}

			var registries []struct {
				GraphQLID string `json:"graphql_id"`
				ID        string `json:"id"`
				Slug      string `json:"slug"`
				Name      string `json:"name"`
				Ecosystem string `json:"ecosystem"`
			}

			if err := json.Unmarshal(bodyBytes, &registries); err != nil {
				return retry.NonRetryableError(fmt.Errorf("error decoding response body: %w", err))
			}

			// Find registry by name
			for _, registry := range registries {
				if registry.Name == state.Name.ValueString() {
					state.Slug = types.StringValue(registry.Slug)
					return nil
				}
			}

			// If we didn't find the registry, it might already be deleted
			return nil
		})
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		// Try to use UUID for lookup if it's available
		var lookupID string

		if !state.Slug.IsNull() && state.Slug.ValueString() != "" {
			lookupID = state.Slug.ValueString()
		} else {
			resp.Diagnostics.AddError(
				"Error deleting registry",
				"Registry slug is not set in state. Please re-import the resource.",
			)
			return nil
		}

		url := fmt.Sprintf("%s/v2/packages/organizations/%s/registries/%s", p.client.restURL, p.client.organization, lookupID)

		// Create the HTTP request
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error creating HTTP request: %w", err))
		}

		// Execute the HTTP request
		resp, err := p.client.http.Do(req)
		if err != nil {
			return retry.RetryableError(fmt.Errorf("error making HTTP request: %w", err))
		}
		defer resp.Body.Close()

		// If the registry was already deleted, consider the delete successful
		if resp.StatusCode == http.StatusNotFound {
			return nil
		}

		// Check for successful response
		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return retry.RetryableError(fmt.Errorf("error deleting registry (status %d): %s", resp.StatusCode, string(bodyBytes)))
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
