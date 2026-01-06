package buildkite

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type clusterSecret struct {
	client *Client
}

type clusterSecretResourceModel struct {
	Id          types.String `tfsdk:"id"`
	Uuid        types.String `tfsdk:"uuid"`
	ClusterId   types.String `tfsdk:"cluster_id"`
	ClusterUuid types.String `tfsdk:"cluster_uuid"`
	Key         types.String `tfsdk:"key"`
	Value       types.String `tfsdk:"value"`
	Description types.String `tfsdk:"description"`
	Policy      types.String `tfsdk:"policy"`
}

func newClusterSecretResource() resource.Resource {
	return &clusterSecret{}
}

func (cs *clusterSecret) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_secret"
}

func (cs *clusterSecret) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	cs.client = req.ProviderData.(*Client)
}

func (cs *clusterSecret) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Cluster Secret is an encrypted secret that can be used by pipelines running on a specific cluster in Buildkite.",
		Attributes: map[string]resource_schema.Attribute{
			"id": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the secret.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the secret.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The GraphQL ID of the Cluster that this secret belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cluster_uuid": resource_schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the Cluster that this secret belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key": resource_schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The key name for the secret. Must start with a letter and contain only letters, numbers, and underscores. Cannot start with `BUILDKITE`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": resource_schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "The secret value to encrypt and store.",
			},
			"description": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description of what this secret is used for.",
			},
			"policy": resource_schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A YAML policy defining access rules for the secret. See [Buildkite documentation](https://buildkite.com/docs/pipelines/cluster-secrets) for the policy format.",
			},
		},
	}
}

func (cs *clusterSecret) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state clusterSecretResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := cs.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *createClusterSecretResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := cs.client.GetOrganizationID()
		if err == nil {
			log.Printf("Creating cluster secret with key %s in cluster %s ...", plan.Key.ValueString(), plan.ClusterId.ValueString())

			r, err = createClusterSecret(ctx,
				cs.client.genqlient,
				*org,
				plan.ClusterId.ValueString(),
				plan.Key.ValueString(),
				plan.Value.ValueString(),
				plan.Description.ValueString(),
				plan.Policy.ValueString(),
			)
		}

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Cluster Secret",
			fmt.Sprintf("Unable to create Cluster Secret: %s", err.Error()),
		)
		return
	}

	state.Id = types.StringValue(r.ClusterSecretCreate.Secret.Id)
	state.Uuid = types.StringValue(r.ClusterSecretCreate.Secret.Uuid)
	state.Key = types.StringValue(r.ClusterSecretCreate.Secret.Key)
	state.ClusterId = types.StringValue(r.ClusterSecretCreate.Secret.Cluster.Id)
	state.ClusterUuid = types.StringValue(r.ClusterSecretCreate.Secret.Cluster.Uuid)
	state.Value = plan.Value

	if r.ClusterSecretCreate.Secret.Description != "" {
		state.Description = types.StringValue(r.ClusterSecretCreate.Secret.Description)
	} else {
		state.Description = types.StringNull()
	}

	if r.ClusterSecretCreate.Secret.Policy != "" {
		state.Policy = types.StringValue(r.ClusterSecretCreate.Secret.Policy)
	} else {
		state.Policy = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (cs *clusterSecret) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterSecretResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := cs.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var r *getNodeResponse
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error

		log.Printf("Reading cluster secret %s ...", state.Id.ValueString())
		r, err = getNode(ctx,
			cs.client.genqlient,
			state.Id.ValueString(),
		)

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Cluster Secret",
			fmt.Sprintf("Unable to read Cluster Secret: %s", err.Error()),
		)
		return
	}

	if secret, ok := r.Node.(*getNodeNodeSecret); ok {
		log.Printf("Found cluster secret with key %s", secret.GetKey())
		state.Key = types.StringValue(secret.GetKey())
		state.ClusterId = types.StringValue(secret.GetCluster().Id)
		state.ClusterUuid = types.StringValue(secret.GetCluster().Uuid)

		if secret.GetDescription() != "" {
			state.Description = types.StringValue(secret.GetDescription())
		} else {
			state.Description = types.StringNull()
		}

		if secret.GetPolicy() != "" {
			state.Policy = types.StringValue(secret.GetPolicy())
		} else {
			state.Policy = types.StringNull()
		}

		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	} else {
		resp.State.RemoveResource(ctx)
	}
}

func (cs *clusterSecret) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan clusterSecretResourceModel

	diagsState := req.State.Get(ctx, &state)
	diagsPlan := req.Plan.Get(ctx, &plan)

	resp.Diagnostics.Append(diagsState...)
	resp.Diagnostics.Append(diagsPlan...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := cs.client.timeouts.Update(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Check if the value changed - if so, we need to call the value update mutation
	if !plan.Value.Equal(state.Value) {
		err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
			org, err := cs.client.GetOrganizationID()
			if err == nil {
				log.Printf("Updating cluster secret value for %s", state.Id.ValueString())
				_, err = updateClusterSecretValue(ctx,
					cs.client.genqlient,
					*org,
					state.Id.ValueString(),
					plan.Value.ValueString(),
				)
			}

			return retryContextError(err)
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to update Cluster Secret value",
				fmt.Sprintf("Unable to update Cluster Secret value: %s", err.Error()),
			)
			return
		}
	}

	// Check if description or policy changed
	if !plan.Description.Equal(state.Description) || !plan.Policy.Equal(state.Policy) {
		var r *updateClusterSecretResponse
		err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
			org, err := cs.client.GetOrganizationID()
			if err == nil {
				log.Printf("Updating cluster secret %s", state.Id.ValueString())

				r, err = updateClusterSecret(ctx,
					cs.client.genqlient,
					*org,
					state.Id.ValueString(),
					plan.Description.ValueString(),
					plan.Policy.ValueString(),
				)
			}

			return retryContextError(err)
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to update Cluster Secret",
				fmt.Sprintf("Unable to update Cluster Secret: %s", err.Error()),
			)
			return
		}

		if r.ClusterSecretUpdate.Secret.Description != "" {
			state.Description = types.StringValue(r.ClusterSecretUpdate.Secret.Description)
		} else {
			state.Description = types.StringNull()
		}

		if r.ClusterSecretUpdate.Secret.Policy != "" {
			state.Policy = types.StringValue(r.ClusterSecretUpdate.Secret.Policy)
		} else {
			state.Policy = types.StringNull()
		}
	} else {
		state.Description = plan.Description
		state.Policy = plan.Policy
	}

	state.Value = plan.Value

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (cs *clusterSecret) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var plan clusterSecretResourceModel

	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := cs.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		org, err := cs.client.GetOrganizationID()
		if err == nil {
			log.Printf("Deleting Cluster Secret %s ...", plan.Id.ValueString())
			_, err = deleteClusterSecret(ctx,
				cs.client.genqlient,
				*org,
				plan.Id.ValueString(),
			)
		}

		return retryContextError(err)
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Cluster Secret",
			fmt.Sprintf("Unable to delete Cluster Secret: %s", err.Error()),
		)
		return
	}
}
