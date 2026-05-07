package buildkite

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
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

type clusterSecretResource struct {
	client *Client
}

type clusterSecretResourceModel struct {
	ID             types.String `tfsdk:"id"`
	ClusterID      types.String `tfsdk:"cluster_id"`
	Key            types.String `tfsdk:"key"`
	Value          types.String `tfsdk:"value"`
	ValueWO        types.String `tfsdk:"value_wo"`
	ValueWOVersion types.String `tfsdk:"value_wo_version"`
	Description    types.String `tfsdk:"description"`
	Policy         types.String `tfsdk:"policy"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
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

func (r *clusterSecretResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("value"),
			path.MatchRoot("value_wo"),
		),
		resourcevalidator.RequiredTogether(
			path.MatchRoot("value_wo"),
			path.MatchRoot("value_wo_version"),
		),
	}
}

func (r *clusterSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			A Cluster Secret is an encrypted key-value pair that can be accessed by agents within a cluster.
			Secrets are encrypted and can only be accessed by agents that match the access policy.

			**Note:** Secret values are write-only in the Buildkite API and cannot be retrieved after they are set.
			Exactly one of value or value_wo must be configured. The value attribute is stored in
			Terraform state so Terraform can detect changes. Use value_wo with value_wo_version
			to pass a secret value without storing it in Terraform plan or state artifacts.
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the cluster secret.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The UUID of the cluster this secret belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The key name for the secret. Must start with a letter and only contain letters, numbers, and underscores. Maximum 255 characters. Must not start with `buildkite` or `bk` (case-insensitive) as these prefixes are reserved.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtMost(255),
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`),
						"must start with a letter and only contain letters, numbers, and underscores",
					),
					reservedSecretKeyPrefixValidator{},
				},
			},
			"value": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "The secret value. Must be less than 8KB. Exactly one of `value` or `value_wo` must be configured. This value is stored in Terraform state; use `value_wo` with `value_wo_version` to avoid storing secret values in state.",
				Validators: []validator.String{
					stringvalidator.LengthAtMost(8192), // 8KB = 8192 bytes
				},
			},
			"value_wo": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
				MarkdownDescription: "Write-only secret value. Must be less than 8KB. Exactly one of `value` or `value_wo` must be configured. This value is not stored in Terraform plan or state artifacts. Pair with `value_wo_version` to trigger secret value updates.",
				Validators: []validator.String{
					stringvalidator.LengthAtMost(8192), // 8KB = 8192 bytes
				},
			},
			"value_wo_version": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Non-secret version identifier for `value_wo`. Required when `value_wo` is configured. Change this value when the write-only secret value changes, for example by using an external secret manager version ID.",
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The time when the secret was last updated.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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

	// Write-only attributes are only available from config, not plan or state.
	var config clusterSecretResourceModel
	diags = req.Config.Get(ctx, &config)
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
			Key:   plan.Key.ValueString(),
			Value: clusterSecretValue(plan, config),
		}
		// Handle optional fields - preserve null vs empty string
		if !plan.Description.IsNull() {
			desc := plan.Description.ValueString()
			secret.Description = &desc
		}
		if !plan.Policy.IsNull() {
			pol := plan.Policy.ValueString()
			secret.Policy = &pol
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
	// Preserve the legacy stateful value since the Buildkite API never returns it.
	existingValue := state.Value

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
	state.Value = existingValue // Keep value from state

	// Handle description - preserve null vs empty string
	if secret.Description == nil {
		state.Description = types.StringNull()
	} else if *secret.Description == "" && state.Description.IsNull() {
		state.Description = types.StringNull()
	} else {
		state.Description = types.StringValue(*secret.Description)
	}

	// Handle policy - preserve null vs empty string
	if secret.Policy == nil {
		state.Policy = types.StringNull()
	} else if *secret.Policy == "" && state.Policy.IsNull() {
		state.Policy = types.StringNull()
	} else {
		state.Policy = types.StringValue(*secret.Policy)
	}

	state.CreatedAt = types.StringValue(secret.CreatedAt)
	state.UpdatedAt = types.StringValue(secret.UpdatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clusterSecretResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Write-only attributes are only available from config, not plan or state.
	var config clusterSecretResourceModel
	diags = req.Config.Get(ctx, &config)
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
		if shouldUpdateClusterSecretValue(plan, state) {
			_, err = r.client.UpdateClusterSecretValue(
				ctx,
				r.client.organization,
				plan.ClusterID.ValueString(),
				plan.ID.ValueString(),
				clusterSecretValue(plan, config),
			)
			if err != nil {
				return retryContextError(err)
			}
		}

		// Update description/policy if changed
		if !plan.Description.Equal(state.Description) || !plan.Policy.Equal(state.Policy) {
			updates := map[string]string{}
			if !plan.Description.Equal(state.Description) {
				if plan.Description.IsNull() {
					updates["description"] = ""
				} else {
					updates["description"] = plan.Description.ValueString()
				}
			}
			if !plan.Policy.Equal(state.Policy) {
				if plan.Policy.IsNull() {
					updates["policy"] = ""
				} else {
					updates["policy"] = plan.Policy.ValueString()
				}
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

	// Preserve created_at from state, Terraform will call Read to refresh updated_at
	plan.CreatedAt = state.CreatedAt
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func clusterSecretValue(plan clusterSecretResourceModel, config clusterSecretResourceModel) string {
	if !config.ValueWO.IsNull() && !config.ValueWO.IsUnknown() {
		return config.ValueWO.ValueString()
	}
	return plan.Value.ValueString()
}

func shouldUpdateClusterSecretValue(plan clusterSecretResourceModel, state clusterSecretResourceModel) bool {
	if !plan.ValueWOVersion.IsNull() || !state.ValueWOVersion.IsNull() {
		return !plan.ValueWOVersion.Equal(state.ValueWOVersion)
	}
	return !plan.Value.Equal(state.Value)
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
		err := r.client.DeleteClusterSecret(ctx, r.client.organization, state.ClusterID.ValueString(), state.ID.ValueString())
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
	// Expected format: cluster_id/secret_id
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid import ID format",
			fmt.Sprintf("Expected format: cluster_id/secret_id, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// reservedSecretKeyPrefixValidator checks that the key doesn't start with reserved prefixes.
// Aligns with Buildkite's backend validation: keys beginning with "buildkite" or "bk"
// (case-insensitive) are rejected.
type reservedSecretKeyPrefixValidator struct{}

func (v reservedSecretKeyPrefixValidator) Description(ctx context.Context) string {
	return "value must not start with 'buildkite' or 'bk' (reserved prefixes, case-insensitive)"
}

func (v reservedSecretKeyPrefixValidator) MarkdownDescription(ctx context.Context) string {
	return "value must not start with `buildkite` or `bk` (reserved prefixes, case-insensitive)"
}

func (v reservedSecretKeyPrefixValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	lower := strings.ToLower(req.ConfigValue.ValueString())
	if strings.HasPrefix(lower, "buildkite") || strings.HasPrefix(lower, "bk") {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid secret key",
			"Secret key must not start with 'buildkite' or 'bk' (reserved prefixes, case-insensitive)",
		)
	}
}
