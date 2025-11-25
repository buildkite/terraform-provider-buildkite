package buildkite

import (
	"context"
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

type clusterSecretResource struct {
	client *Client
}

type clusterSecretResourceModel struct {
	ID          types.String `tfsdk:"id"`
	ClusterID   types.String `tfsdk:"cluster_id"`
	Key         types.String `tfsdk:"key"`
	Value       types.String `tfsdk:"value"`
	Description types.String `tfsdk:"description"`
	Policy      types.String `tfsdk:"policy"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
}

func newClusterSecretResource() resource.Resource {
	return &clusterSecretResource{}
}

func (r *clusterSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_secret"
}

func (r *clusterSecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*Client)
}

func (r *clusterSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			A Cluster Secret is an encrypted key-value pair that can be accessed by agents within a cluster.
			Secrets are encrypted and can only be accessed by agents that match the access policy.
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the cluster secret.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the cluster this secret belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The key name for the secret. Must start with a letter and only contain letters, numbers, and underscores. Maximum 255 characters.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "The secret value. Must be less than 8KB.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description of what this secret is for.",
			},
			"policy": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "YAML access policy defining which pipelines and branches can access this secret.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The time when the secret was created.",
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The time when the secret was last updated.",
			},
		},
	}
}

func (r *clusterSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterSecretResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := r.client.timeouts.Create(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created *ClusterSecret
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		secret := &ClusterSecret{
			Key:         plan.Key.ValueString(),
			Value:       plan.Value.ValueString(),
			Description: plan.Description.ValueString(),
			Policy:      plan.Policy.ValueString(),
		}

		created, err = r.client.CreateClusterSecret(ctx, r.client.organization, plan.ClusterID.ValueString(), secret)
		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create cluster secret",
			fmt.Sprintf("Unable to create cluster secret: %s", err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.CreatedAt = types.StringValue(created.CreatedAt)
	plan.UpdatedAt = types.StringValue(created.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterSecretResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := r.client.timeouts.Read(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var secret *ClusterSecret
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		secret, err = r.client.GetClusterSecret(ctx, r.client.organization, state.ClusterID.ValueString(), state.ID.ValueString())
		return retryContextError(err)
	})

	if err != nil {
		if strings.Contains(err.Error(), "status: 404") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to read cluster secret",
			fmt.Sprintf("Unable to read cluster secret: %s", err.Error()),
		)
		return
	}

	state.Key = types.StringValue(secret.Key)
	state.Description = types.StringValue(secret.Description)
	state.Policy = types.StringValue(secret.Policy)
	state.CreatedAt = types.StringValue(secret.CreatedAt)
	state.UpdatedAt = types.StringValue(secret.UpdatedAt)
	// Note: Value is never returned by API, so we keep the plan value

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clusterSecretResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state clusterSecretResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := r.client.timeouts.Update(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		// Update value if changed
		if !plan.Value.Equal(state.Value) {
			_, err = r.client.UpdateClusterSecretValue(
				ctx,
				r.client.organization,
				plan.ClusterID.ValueString(),
				plan.ID.ValueString(),
				plan.Value.ValueString(),
			)
			if err != nil {
				return retryContextError(err)
			}
		}

		// Update description/policy if changed
		if !plan.Description.Equal(state.Description) || !plan.Policy.Equal(state.Policy) {
			updates := map[string]string{}
			if !plan.Description.Equal(state.Description) {
				updates["description"] = plan.Description.ValueString()
			}
			if !plan.Policy.Equal(state.Policy) {
				updates["policy"] = plan.Policy.ValueString()
			}

			_, err = r.client.UpdateClusterSecret(
				ctx,
				r.client.organization,
				plan.ClusterID.ValueString(),
				plan.ID.ValueString(),
				updates,
			)
			if err != nil {
				return retryContextError(err)
			}
		}

		return nil
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update cluster secret",
			fmt.Sprintf("Unable to update cluster secret: %s", err.Error()),
		)
		return
	}

	// Read the updated secret to get the new updated_at timestamp
	var updated *ClusterSecret
	err = retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		updated, err = r.client.GetClusterSecret(ctx, r.client.organization, plan.ClusterID.ValueString(), plan.ID.ValueString())
		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read updated cluster secret",
			fmt.Sprintf("Unable to read updated cluster secret: %s", err.Error()),
		)
		return
	}

	plan.UpdatedAt = types.StringValue(updated.UpdatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterSecretResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := r.client.timeouts.Delete(ctx, DefaultTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		err = r.client.DeleteClusterSecret(ctx, r.client.organization, state.ClusterID.ValueString(), state.ID.ValueString())
		return retryContextError(err)
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete cluster secret",
			fmt.Sprintf("Unable to delete cluster secret: %s", err.Error()),
		)
		return
	}
}

func (r *clusterSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
