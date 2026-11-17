package planmodifier

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type useNonEmptyStateIfUnchangedModifier struct {
	attr string
}

// Description implements planmodifier.String.
func (useNonEmptyStateIfUnchangedModifier) Description(context.Context) string {
	return "Once set to a non-empty value, the value of this attribute in state will only change if the dependent attribute changes. An empty string in state is treated like null and planned as unknown."
}

// MarkdownDescription implements planmodifier.String.
func (useNonEmptyStateIfUnchangedModifier) MarkdownDescription(context.Context) string {
	return "Once set to a non-empty value, the value of this attribute in state will only change if the dependent attribute changes. An empty string in state is treated like null and planned as unknown."
}

// PlanModifyString implements planmodifier.String.
func (mod useNonEmptyStateIfUnchangedModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// "" can mean the API withheld the value from the token that read it, so it must not be copied into the plan
	// either or an apply that returns the value would be inconsistent with it
	if req.StateValue.IsNull() || req.StateValue.ValueString() == "" {
		return
	}

	if !req.PlanValue.IsUnknown() {
		return
	}

	if req.ConfigValue.IsUnknown() {
		return
	}

	var stateValue, planValue types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(mod.attr), &stateValue)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(mod.attr), &planValue)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// a change to the dependent attribute, including one that is not known until apply, can change this value too
	if !planValue.Equal(stateValue) {
		return
	}

	resp.PlanValue = req.StateValue
}

// UseNonEmptyStateIfUnchanged combines UseNonEmptyStateForUnknown and UseStateIfUnchanged: it keeps a non-empty
// value from state unless the dependent attribute changes, and leaves the plan unknown when state holds an empty
// string.
func UseNonEmptyStateIfUnchanged(attr string) planmodifier.String {
	return useNonEmptyStateIfUnchangedModifier{attr}
}
