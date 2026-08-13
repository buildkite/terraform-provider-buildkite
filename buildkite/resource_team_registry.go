package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type teamRegistryModel struct {
	ID          types.String `tfsdk:"id"`
	UUID        types.String `tfsdk:"uuid"`
	RegistryID  types.String `tfsdk:"registry_id"`
	TeamID      types.String `tfsdk:"team_id"`
	AccessLevel types.String `tfsdk:"access_level"`
}

type teamRegistryResource struct {
	client *Client
}

func newTeamRegistryResource() resource.Resource {
	return &teamRegistryResource{}
}

func (*teamRegistryResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_registry"
}

func (tr *teamRegistryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	tr.client = req.ProviderData.(*Client)
}

func (tr *teamRegistryResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage team access to a package registry. The teams a registry is created with (`team_ids` on `buildkite_registry`) already have access; use this resource to grant access to other teams or to manage a team's access level.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the team-registry relationship.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the team-registry relationship.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"registry_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the registry.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"team_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the team.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"access_level": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The access level the team has on the registry. Either `READ_ONLY`, `READ_AND_WRITE` or `READ_WRITE_AND_ADMIN`.",
				Validators: []validator.String{
					stringvalidator.OneOf("READ_ONLY", "READ_AND_WRITE", "READ_WRITE_AND_ADMIN"),
				},
			},
		},
	}
}

func (tr *teamRegistryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state teamRegistryModel

	diags := req.Plan.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := tr.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Adding team %s to registry %s ...", state.TeamID.ValueString(), state.RegistryID.ValueString())
	var r *createTeamRegistryResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		r, err = createTeamRegistry(ctx,
			tr.client.genqlient,
			state.TeamID.ValueString(),
			state.RegistryID.ValueString(),
			RegistryAccessLevels(state.AccessLevel.ValueString()),
		)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create team registry",
			fmt.Sprintf("Unable to create team registry: %s", err.Error()),
		)
		return
	}

	state.ID = types.StringValue(r.TeamRegistryCreate.TeamRegistry.Id)
	state.UUID = types.StringValue(r.TeamRegistryCreate.TeamRegistry.TeamRegistryUuid)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (tr *teamRegistryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamRegistryModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := tr.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Reading team registry with ID %s ...", state.ID.ValueString())
	var r *getNodeResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		r, err = getNode(ctx,
			tr.client.genqlient,
			state.ID.ValueString(),
		)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read team registry",
			fmt.Sprintf("Unable to read team registry: %s", err.Error()),
		)
		return
	}

	if teamRegistryNode, ok := r.GetNode().(*getNodeNodeTeamRegistry); ok {
		if teamRegistryNode == nil {
			resp.Diagnostics.AddError(
				"Unable to get team registry",
				"Error getting team registry: nil response",
			)
			return
		}
		updateTeamRegistryResource(&state, teamRegistryNode.TeamRegistryFields)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		// team registry was removed - remove from state
		resp.Diagnostics.AddWarning("Team registry not found", "Removing team registry from state")
		resp.State.RemoveResource(ctx)
		return
	}
}

func (tr *teamRegistryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (tr *teamRegistryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state teamRegistryModel
	var accessLevel string

	diagsState := req.State.Get(ctx, &state)
	diagsAccessLevel := req.Plan.GetAttribute(ctx, path.Root("access_level"), &accessLevel)

	resp.Diagnostics.Append(diagsState...)
	resp.Diagnostics.Append(diagsAccessLevel...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := tr.client.timeouts.Update(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Updating team %s in registry %s to %s ...", state.TeamID.ValueString(), state.RegistryID.ValueString(), accessLevel)
	var r *updateTeamRegistryResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		r, err = updateTeamRegistry(ctx,
			tr.client.genqlient,
			state.ID.ValueString(),
			RegistryAccessLevels(accessLevel),
		)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update team registry",
			fmt.Sprintf("Unable to update team registry: %s", err.Error()),
		)
		return
	}

	state.AccessLevel = types.StringValue(string(r.TeamRegistryUpdate.TeamRegistry.RegistryAccessLevel))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (tr *teamRegistryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamRegistryModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := tr.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Deleting team %s's access to registry %s ...", state.TeamID.ValueString(), state.RegistryID.ValueString())
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		_, err := deleteTeamRegistry(ctx,
			tr.client.genqlient,
			state.ID.ValueString(),
		)
		if err != nil && isResourceNotFoundError(err) {
			return nil
		}

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete team registry",
			fmt.Sprintf("Unable to delete team registry: %s", err.Error()),
		)
		return
	}
}

func updateTeamRegistryResource(trm *teamRegistryModel, trf TeamRegistryFields) {
	trm.ID = types.StringValue(trf.Id)
	trm.UUID = types.StringValue(trf.TeamRegistryUuid)
	trm.TeamID = types.StringValue(trf.Team.Id)
	trm.RegistryID = types.StringValue(trf.Registry.Id)
	trm.AccessLevel = types.StringValue(string(trf.RegistryAccessLevel))
}
